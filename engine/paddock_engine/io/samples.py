"""Raw samples of one corner pass, as the agent recorded them (contracts/spool_pass.schema.json)."""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path

import duckdb
import numpy as np
import numpy.typing as npt

FloatArray = npt.NDArray[np.float64]

WHEELS = ("fl", "fr", "rl", "rr")
SCALAR_COLUMNS = (
    "t", "d", "v", "gas", "brake", "steer", "lat_g", "lon_g", "yaw", "out", "rh_f", "rh_r",
)  # fmt: skip
WHEEL_COLUMNS = ("sa", "sr", "load")
COLUMN_NAMES = SCALAR_COLUMNS + tuple(f"{c}_{w}" for c in WHEEL_COLUMNS for w in WHEELS)


@dataclass(frozen=True)
class PassSamples:
    """Column arrays of equal length. Per-wheel arrays are (n, 4): FL, FR, RL, RR."""

    t: FloatArray  # seconds since the pass started
    d: FloatArray  # metres from the start line
    v: FloatArray  # m/s
    gas: FloatArray
    brake: FloatArray
    steer: FloatArray  # steering wheel angle, degrees
    lat_g: FloatArray
    lon_g: FloatArray
    yaw: FloatArray  # rad/s
    out: FloatArray  # wheels outside the track
    rh_f: FloatArray
    rh_r: FloatArray
    slip_angle: FloatArray  # degrees
    slip_ratio: FloatArray
    load: FloatArray  # N

    @property
    def n(self) -> int:
        return len(self.t)

    @classmethod
    def from_columns(cls, columns: Mapping[str, Sequence[float]]) -> "PassSamples":
        missing = [name for name in COLUMN_NAMES if name not in columns]
        if missing:
            raise ValueError(f"missing sample columns: {missing}")
        lengths = {len(columns[name]) for name in COLUMN_NAMES}
        if len(lengths) != 1:
            raise ValueError(f"sample columns differ in length: {sorted(lengths)}")

        def scalar(name: str) -> FloatArray:
            return np.asarray(columns[name], dtype=np.float64)

        def per_wheel(prefix: str) -> FloatArray:
            return np.column_stack([scalar(f"{prefix}_{wheel}") for wheel in WHEELS])

        return cls(
            **{name: scalar(name) for name in SCALAR_COLUMNS},
            slip_angle=per_wheel("sa"),
            slip_ratio=per_wheel("sr"),
            load=per_wheel("load"),
        )


def read_session_samples(parquet_dir: Path) -> dict[int, PassSamples]:
    """Every pass with raw samples in a session's Parquet parts, keyed by pass sequence number.

    Core's ingest writes one part per spool file under this directory (docs/adr/0007).
    """
    parts = sorted(parquet_dir.glob("*.parquet"))
    if not parts:
        return {}
    select = ", ".join(COLUMN_NAMES)
    connection = duckdb.connect()
    try:
        table = connection.execute(
            f"SELECT seq, {select} FROM read_parquet(?) ORDER BY seq, t",
            [[str(part) for part in parts]],
        ).fetchnumpy()
    finally:
        connection.close()
    passes: dict[int, PassSamples] = {}
    seq = table["seq"]
    for value in np.unique(seq):
        rows = seq == value
        passes[int(value)] = PassSamples.from_columns(
            {name: table[name][rows] for name in COLUMN_NAMES}
        )
    return passes
