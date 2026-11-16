"""tools/prepare_test_layout.py: the copy gets physics access, the original is never touched."""

import importlib.util
import sys
from pathlib import Path

import pytest

TOOL = Path(__file__).resolve().parents[2] / "tools" / "prepare_test_layout.py"
_spec = importlib.util.spec_from_file_location("prepare_test_layout", TOOL)
assert _spec and _spec.loader
layout_tool = importlib.util.module_from_spec(_spec)
sys.modules["prepare_test_layout"] = layout_tool
_spec.loader.exec_module(layout_tool)

SURFACES = (
    "[SURFACE_0]\nKEY=GRASS\nWAV_PITCH=0.0\nFRICTION=0.6\n\n[SURFACE_1]\nKEY=ROAD\nWAV_PITCH=1.3\n"
)


def make_track(root: Path) -> Path:
    track = root / "fn_test"
    (track / "lay" / "data").mkdir(parents=True)
    (track / "lay" / "data" / "surfaces.ini").write_text(SURFACES)
    (track / "lay" / "ai").mkdir()
    (track / "lay" / "ai" / "fast_lane.ai").write_bytes(b"\x01\x02")
    (track / "models_lay.ini").write_text("[MODEL_0]\nFILE=a.kn5\n")
    (track / "ui" / "lay").mkdir(parents=True)
    info = '﻿{\n  "name": "Test Track",\n  "year": 1}'  # AC's files often start with a BOM
    (track / "ui" / "lay" / "ui_track.json").write_text(info, encoding="utf-8")
    return track


@pytest.mark.parametrize(
    ("surfaces", "expected_pitch"),
    [
        (SURFACES, "WAV_PITCH=extended-0"),
        ("[SURFACE_0]\nKEY=GRASS\n\n[SURFACE_1]\nKEY=ROAD\n", "WAV_PITCH=extended-0"),
    ],
)
def test_enable_app_physics_turns_on_extended_physics_in_surface_0_only(surfaces, expected_pitch):
    text = layout_tool.enable_app_physics(surfaces)
    first = text.split("[SURFACE_1]")[0]
    assert expected_pitch in first
    assert "[SURFACE_1]" in text and text.count("extended-0") == 1
    assert "[_SCRIPTING_PHYSICS]\nALLOW_APPS=1" in text


def test_enable_app_physics_updates_an_existing_permission_section():
    text = layout_tool.enable_app_physics(SURFACES + "\n[_SCRIPTING_PHYSICS]\nALLOW_APPS=0\n")
    assert text.count("[_SCRIPTING_PHYSICS]") == 1 and "ALLOW_APPS=1" in text


def test_enable_app_physics_rejects_a_file_without_surface_0():
    with pytest.raises(ValueError):
        layout_tool.enable_app_physics("[SURFACE_1]\nKEY=ROAD\n")


def test_prepare_copies_the_layout_and_leaves_the_original_byte_for_byte(tmp_path):
    track = make_track(tmp_path)
    copy = layout_tool.prepare(track, "lay")
    assert copy == track / "lay_paddock"
    assert (track / "lay" / "data" / "surfaces.ini").read_text() == SURFACES
    assert "ALLOW_APPS=1" in (copy / "data" / "surfaces.ini").read_text()
    assert (copy / "ai" / "fast_lane.ai").read_bytes() == b"\x01\x02"
    assert (track / "models_lay_paddock.ini").read_text() == "[MODEL_0]\nFILE=a.kn5\n"
    info = (track / "ui" / "lay_paddock" / "ui_track.json").read_text(encoding="utf-8")
    assert '"Test Track (Paddock test)"' in info


def test_prepare_refuses_to_overwrite_an_existing_copy(tmp_path):
    track = make_track(tmp_path)
    layout_tool.prepare(track, "lay")
    with pytest.raises(FileExistsError):
        layout_tool.prepare(track, "lay")


def test_remove_deletes_only_what_prepare_made(tmp_path):
    track = make_track(tmp_path)
    layout_tool.prepare(track, "lay")
    layout_tool.remove(track, "lay")
    assert not (track / "lay_paddock").exists()
    assert not (track / "models_lay_paddock.ini").exists()
    assert not (track / "ui" / "lay_paddock").exists()
    assert (track / "lay" / "data" / "surfaces.ini").exists()


def test_remove_refuses_a_folder_without_the_marker(tmp_path):
    track = make_track(tmp_path)
    (track / "lay_paddock").mkdir()
    with pytest.raises(FileNotFoundError):
        layout_tool.remove(track, "lay")
    assert (track / "lay_paddock").exists()
