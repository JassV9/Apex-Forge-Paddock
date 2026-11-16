"""Adds a copy of a track layout on which the Paddock agent may drive the car (Ghost Driver).

    python tools/prepare_test_layout.py fn_cota layout_f1_2026            # add the copy
    python tools/prepare_test_layout.py fn_cota layout_f1_2026 --remove   # delete it again

CSP lets a Lua app hand the player's car to AC's AI only on a track that enables extended
physics and grants apps physics access in its surfaces.ini (docs/adr/0014-ghost-driver.md).
Instead of editing the original layout, this copies it to `<layout>_paddock`, changes the copy
only, and marks it so that --remove deletes nothing it did not make. The original layout, its
files and its online checksum stay as they were.
"""

import argparse
import re
import shutil
import sys
from pathlib import Path

DEFAULT_AC_ROOT = Path(r"C:\Program Files (x86)\Steam\steamapps\common\assettocorsa")
SUFFIX = "_paddock"
MARKER = "paddock_test_layout.txt"
PERMISSIONS = "\n[_SCRIPTING_PHYSICS]\nALLOW_APPS=1\n"


def enable_app_physics(surfaces: str) -> str:
    """Return surfaces.ini text with extended physics on and Lua apps allowed physics access."""
    match = re.search(r"^\[SURFACE_0\][^\[]*", surfaces, flags=re.MULTILINE)
    if not match:
        raise ValueError("surfaces.ini has no [SURFACE_0] section")
    section = match.group(0)
    if re.search(r"^WAV_PITCH=", section, flags=re.MULTILINE):
        changed = re.sub(r"^WAV_PITCH=.*$", "WAV_PITCH=extended-0", section, flags=re.MULTILINE)
    else:
        changed = section.rstrip("\n") + "\nWAV_PITCH=extended-0\n\n"
    text = surfaces[: match.start()] + changed + surfaces[match.end() :]
    if "[_SCRIPTING_PHYSICS]" in text:
        return re.sub(r"^ALLOW_APPS=.*$", "ALLOW_APPS=1", text, flags=re.MULTILINE)
    return text.rstrip("\n") + "\n" + PERMISSIONS


def _targets(track: Path, layout: str) -> dict[str, tuple[Path, Path]]:
    copy = layout + SUFFIX
    pairs = {
        "layout": (track / layout, track / copy),
        "models": (track / f"models_{layout}.ini", track / f"models_{copy}.ini"),
        "vao": (track / f"models_{layout}.vao-patch", track / f"models_{copy}.vao-patch"),
        "ui": (track / "ui" / layout, track / "ui" / copy),
    }
    return {name: pair for name, pair in pairs.items() if pair[0].exists()}


def _rename_ui(ui_dir: Path) -> None:
    info = ui_dir / "ui_track.json"
    if not info.exists():
        return
    text = info.read_text(encoding="utf-8-sig")
    text = re.sub(r'("name"\s*:\s*")([^"]*)(")', r"\1\2 (Paddock test)\3", text, count=1)
    info.write_text(text, encoding="utf-8")


def prepare(track: Path, layout: str) -> Path:
    targets = _targets(track, layout)
    if "layout" not in targets:
        raise FileNotFoundError(f"{track / layout} does not exist")
    copy_dir = targets["layout"][1]
    if copy_dir.exists():
        raise FileExistsError(f"{copy_dir} already exists: remove it first with --remove")
    for source, target in targets.values():
        if source.is_dir():
            shutil.copytree(source, target)
        else:
            shutil.copy2(source, target)
    surfaces = copy_dir / "data" / "surfaces.ini"
    text = surfaces.read_text(encoding="utf-8", errors="surrogateescape")
    surfaces.write_text(enable_app_physics(text), encoding="utf-8", errors="surrogateescape")
    if "ui" in targets:
        _rename_ui(targets["ui"][1])
    (copy_dir / MARKER).write_text(f"Copy of {layout} made by tools/prepare_test_layout.py\n")
    return copy_dir


def remove(track: Path, layout: str) -> Path:
    copy_dir = track / (layout + SUFFIX)
    if not (copy_dir / MARKER).exists():
        raise FileNotFoundError(f"{copy_dir} was not made by this tool; nothing removed")
    for _, target in _targets(track, layout).values():
        if target.is_dir():
            shutil.rmtree(target)
        elif target.exists():
            target.unlink()
    return copy_dir


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("track")
    parser.add_argument("layout")
    parser.add_argument("--remove", action="store_true")
    parser.add_argument("--ac-root", type=Path, default=DEFAULT_AC_ROOT)
    args = parser.parse_args()
    track = args.ac_root / "content" / "tracks" / args.track
    try:
        done = remove(track, args.layout) if args.remove else prepare(track, args.layout)
    except (OSError, ValueError) as error:
        print(f"failed: {error}", file=sys.stderr)
        return 1
    print(f"{'removed' if args.remove else 'made'} {done}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
