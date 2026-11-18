"""Copies the in-game agent into Assetto Corsa's Lua apps folder.

    python tools/install_agent.py                      # default Steam location
    python tools/install_agent.py --ac-root "D:\\Games\\assettocorsa"

Only the app itself is copied (PaddockAgent.lua, manifest.ini, src/); tests and fixtures stay
in the repository. An existing PaddockAgent folder is replaced; nothing else is touched.
The copy gets src/build.lua with a hash of what was copied: every session line names the build
it was recorded with (docs/adr/0028-run-context-and-ai-provenance.md).
Then enable "Paddock Agent" in the in-game apps bar (Custom Shaders Patch required).
"""

import argparse
import hashlib
import shutil
import sys
from pathlib import Path

AGENT_DIR = Path(__file__).resolve().parents[1] / "agent"
APP_NAME = "PaddockAgent"
DEFAULT_AC_ROOT = Path(r"C:\Program Files (x86)\Steam\steamapps\common\assettocorsa")


def build_hash(agent_dir: Path) -> str:
    """The first 12 hex digits of a SHA-256 over every file the install copies, path and bytes:
    the same sources give the same build, any change gives another."""
    files = [agent_dir / f"{APP_NAME}.lua", agent_dir / "manifest.ini"]
    files += sorted(path for path in (agent_dir / "src").rglob("*") if path.is_file())
    digest = hashlib.sha256()
    for path in files:
        digest.update(path.relative_to(agent_dir).as_posix().encode("utf-8") + b"\0")
        digest.update(path.read_bytes() + b"\0")
    return digest.hexdigest()[:12]


def install(ac_root: Path, agent_dir: Path = AGENT_DIR) -> Path:
    apps = ac_root / "apps" / "lua"
    if not apps.is_dir():
        raise FileNotFoundError(f"{apps} does not exist: is this an Assetto Corsa folder with CSP?")
    target = apps / APP_NAME
    if target.exists():
        shutil.rmtree(target)
    target.mkdir()
    shutil.copy2(agent_dir / f"{APP_NAME}.lua", target)
    shutil.copy2(agent_dir / "manifest.ini", target)
    shutil.copytree(agent_dir / "src", target / "src")
    build = build_hash(agent_dir)
    (target / "src" / "build.lua").write_text(f'return {{ hash = "{build}" }}\n', encoding="utf-8")
    return target


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--ac-root", type=Path, default=DEFAULT_AC_ROOT)
    args = parser.parse_args()
    try:
        target = install(args.ac_root)
    except OSError as error:
        print(f"install failed: {error}", file=sys.stderr)
        return 1
    print(f"installed to {target}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
