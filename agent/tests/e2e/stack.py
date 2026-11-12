"""Starts the real backend for the end-to-end test: a fresh database, the engine and core.

Core is built from source with `go build`; the engine runs from its own virtual environment.
Both listen on free local ports and are stopped when the test module ends.
"""

import json
import os
import shutil
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit, urlunsplit

import psycopg

REPO = Path(__file__).resolve().parents[3]
DATABASE_ENV = "PADDOCK_TEST_DATABASE_URL"
REQUIRED_ENV = "PADDOCK_E2E_REQUIRED"  # set in CI: missing tools fail the run instead of skipping
E2E_DATABASE = "paddock_e2e"
START_TIMEOUT_S = 90


def missing() -> str | None:
    """Why the stack cannot be started here, or None when it can."""
    if not os.environ.get(DATABASE_ENV):
        return f"{DATABASE_ENV} is not set"
    if shutil.which("go") is None:
        return "the Go toolchain is not on PATH"
    if not engine_python().exists():
        return "the engine has no virtual environment (run `uv sync` in engine/)"
    return None


def engine_python() -> Path:
    folder = "Scripts/python.exe" if sys.platform == "win32" else "bin/python"
    return REPO / "engine" / ".venv" / folder


def free_port() -> int:
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return int(probe.getsockname()[1])


def _ipv4(url: str) -> str:
    """`localhost` as 127.0.0.1. libpq tries ::1 first, and where the database only listens on
    IPv4 (Docker Desktop on Windows) that attempt hangs for two minutes before falling back."""
    parts = urlsplit(url)
    if parts.hostname != "localhost":
        return url
    return urlunsplit(parts._replace(netloc=parts.netloc.replace("@localhost", "@127.0.0.1")))


def fresh_database() -> str:
    """Drop and create the e2e database next to the test database; returns its URL."""
    base = _ipv4(os.environ[DATABASE_ENV])
    with psycopg.connect(base, autocommit=True) as connection:
        connection.execute(f'DROP DATABASE IF EXISTS "{E2E_DATABASE}" WITH (FORCE)')
        connection.execute(f'CREATE DATABASE "{E2E_DATABASE}"')
    return urlunsplit(urlsplit(base)._replace(path="/" + E2E_DATABASE))


@dataclass
class Stack:
    """A running core and engine. `core_url` is the REST API the dashboard uses."""

    core_url: str
    processes: list[subprocess.Popen[bytes]] = field(default_factory=list)
    logs: list[Path] = field(default_factory=list)

    def request(self, method: str, path: str, body: Any = None) -> tuple[int, Any]:
        data = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(self.core_url + path, data=data, method=method)
        request.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(request, timeout=120) as response:
                return response.status, json.loads(response.read())
        except urllib.error.HTTPError as error:
            return error.code, json.loads(error.read() or b"{}")

    def get(self, path: str) -> Any:
        status, body = self.request("GET", path)
        assert status == 200, f"GET {path} -> {status}: {body}"
        return body

    def post(self, path: str, body: Any = None) -> Any:
        status, reply = self.request("POST", path, body)
        assert status in (200, 201), f"POST {path} -> {status}: {reply}\n{self.tail()}"
        return reply

    def tail(self) -> str:
        return "\n".join(
            f"--- {log.name} ---\n" + "\n".join(log.read_text(errors="replace").splitlines()[-25:])
            for log in self.logs
        )

    def stop(self) -> None:
        for process in self.processes:
            process.terminate()
        for process in self.processes:
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()


def _wait_healthy(url: str, process: subprocess.Popen[bytes], log: Path) -> None:
    deadline = time.monotonic() + START_TIMEOUT_S
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"{log.name} exited:\n{log.read_text(errors='replace')[-3000:]}")
        try:
            with urllib.request.urlopen(url + "/health", timeout=2) as response:
                if response.status == 200:
                    return
        except OSError:
            time.sleep(0.2)
    raise RuntimeError(f"{url} did not answer:\n{log.read_text(errors='replace')[-3000:]}")


def _spawn(stack: Stack, command: list[str], cwd: Path, env: dict[str, str], log: Path) -> None:
    with log.open("wb") as output:
        process = subprocess.Popen(
            command, cwd=cwd, env=os.environ | env, stdout=output, stderr=subprocess.STDOUT
        )
    stack.processes.append(process)
    stack.logs.append(log)


def start(work: Path, game_documents: Path) -> Stack:
    """Build and start core and the engine. `game_documents` is the agent's Documents folder:
    core reads its spool and writes its outbox there, as it does next to the real game."""
    binary = work / ("paddock.exe" if sys.platform == "win32" else "paddock")
    subprocess.run(
        ["go", "build", "-o", str(binary), "./cmd/paddock"],
        cwd=REPO / "core",
        check=True,
    )
    engine_url = f"http://127.0.0.1:{free_port()}"
    stack = Stack(core_url=f"http://127.0.0.1:{free_port()}")
    try:
        uvicorn = [
            "-m",
            "uvicorn",
            "paddock_engine.app:create_app",
            "--factory",
            "--host",
            "127.0.0.1",
        ]
        port = engine_url.rsplit(":", 1)[1]
        _spawn(
            stack,
            [str(engine_python()), *uvicorn, "--port", port],
            REPO / "engine",
            {"PADDOCK_LLM_PROVIDER": "none"},  # the test never talks to a hosted model
            work / "engine.log",
        )
        _wait_healthy(engine_url, stack.processes[0], stack.logs[0])
        core_env = {
            "PADDOCK_HTTP_ADDR": stack.core_url.removeprefix("http://"),
            "PADDOCK_DATABASE_URL": fresh_database(),
            "PADDOCK_SPOOL_DIR": str(game_documents / "paddock" / "spool"),
            "PADDOCK_OUTBOX_DIR": str(game_documents / "paddock" / "outbox"),
            "PADDOCK_DATA_DIR": str(work / "data"),
            "PADDOCK_ENGINE_URL": engine_url,
        }
        _spawn(stack, [str(binary)], work, core_env, work / "core.log")
        _wait_healthy(stack.core_url, stack.processes[1], stack.logs[1])
    except BaseException:
        stack.stop()
        raise
    return stack
