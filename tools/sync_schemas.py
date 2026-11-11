"""Mirrors contracts/*.schema.json into a directory of generated copies (used by `make gen`).

    python tools/sync_schemas.py <contracts dir> <target dir>

Copies that no longer have a source are removed, so the target never drifts.
"""

import shutil
import sys
from pathlib import Path


def sync(source: Path, target: Path) -> None:
    target.mkdir(parents=True, exist_ok=True)
    wanted = {path.name for path in source.glob("*.schema.json")}
    for stale in target.glob("*.schema.json"):
        if stale.name not in wanted:
            stale.unlink()
    for name in sorted(wanted):
        shutil.copyfile(source / name, target / name)


if __name__ == "__main__":
    sync(Path(sys.argv[1]), Path(sys.argv[2]))
