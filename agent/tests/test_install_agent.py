"""tools/install_agent.py: the installed copy names its build, and the build changes with the
sources (docs/adr/0028-run-context-and-ai-provenance.md)."""

import importlib.util
import shutil
import sys
from pathlib import Path

from harness import AGENT_DIR

TOOL = Path(__file__).resolve().parents[2] / "tools" / "install_agent.py"
_spec = importlib.util.spec_from_file_location("install_agent", TOOL)
assert _spec and _spec.loader
installer = importlib.util.module_from_spec(_spec)
sys.modules["install_agent"] = installer
_spec.loader.exec_module(installer)


def copy_of_agent(root: Path) -> Path:
    agent = root / "agent"
    agent.mkdir()
    shutil.copy2(AGENT_DIR / "PaddockAgent.lua", agent)
    shutil.copy2(AGENT_DIR / "manifest.ini", agent)
    shutil.copytree(AGENT_DIR / "src", agent / "src")
    return agent


def test_the_installed_copy_names_its_build(tmp_path: Path) -> None:
    (tmp_path / "ac" / "apps" / "lua").mkdir(parents=True)
    agent = copy_of_agent(tmp_path)

    target = installer.install(tmp_path / "ac", agent)

    build = (target / "src" / "build.lua").read_text(encoding="utf-8")
    assert build == f'return {{ hash = "{installer.build_hash(agent)}" }}\n'
    assert not (agent / "src" / "build.lua").exists(), "the sources are left as they were"


def test_the_build_changes_with_any_source_and_only_then(tmp_path: Path) -> None:
    agent = copy_of_agent(tmp_path)
    first = installer.build_hash(agent)

    assert installer.build_hash(agent) == first and len(first) == 12
    with (agent / "src" / "core" / "aitune.lua").open("a", encoding="utf-8") as source:
        source.write("\n-- changed\n")
    assert installer.build_hash(agent) != first
