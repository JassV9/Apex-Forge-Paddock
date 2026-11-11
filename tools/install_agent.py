"""Copies the in-game agent into Assetto Corsa's Lua apps folder.

    python tools/install_agent.py                      # default Steam location
    python tools/install_agent.py --ac-root "D:\\Games\\assettocorsa"

Only the app itself is copied (PaddockAgent.lua, manifest.ini, src/); tests and fixtures stay
in the repository. An existing PaddockAgent folder is replaced; nothing else is touched.
Then enable "Paddock Agent" in the in-game apps bar (Custom Shaders Patch required).
"""

import argparse
import shutil
import sys
from pathlib import Path

AGENT_DIR = Path(__file__).resolve().parents[1] / "agent"
APP_NAME = "PaddockAgent"
DEFAULT_AC_ROOT = Path(r"C:\Program Files (x86)\Steam\steamapps\common\assettocorsa")


def install(ac_root: Path) -> Path:
    apps = ac_root / "apps" / "lua"
    if not apps.is_dir():
        raise FileNotFoundError(f"{apps} does not exist: is this an Assetto Corsa folder with CSP?")
    target = apps / APP_NAME
    if target.exists():
        shutil.rmtree(target)
    target.mkdir()
    shutil.copy2(AGENT_DIR / f"{APP_NAME}.lua", target)
    shutil.copy2(AGENT_DIR / "manifest.ini", target)
    shutil.copytree(AGENT_DIR / "src", target / "src")
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
