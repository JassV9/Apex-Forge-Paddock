# ADR 0013: One workspace with telemetry charts, uPlot, and a read path for raw samples

Status: accepted (2026-11-15)

## Context

The dashboard was five separate pages (sessions, ledger, garage, propose, and one page per
session or experiment) and could not show what a session recorded: core had no route for the
passes of a session or for their raw samples, so nobody could look at a corner, compare a
pass with the best one, or see the conditions it was driven in. The work is splitting: the
in-game app manages setups and runs experiments; the dashboard is where telemetry and
analysis are looked at.

Two things were missing: a way to read passes and samples, and a way to draw several 60 Hz
traces of a corner quickly and legibly in the browser.

## Decision

**One workspace.** A rail of sessions on the left (car, track, time, passes, grouped by day)
and the selected session on the right with five tabs: Overview, Telemetry, Diagnosis,
Experiments and Garage. Experiments and the garage are per car and show the selected
session's car. Proposing stays in the web for now, as a panel inside the Experiments tab.
Everything is in the location hash (`#/sessions/2/telemetry?corner=5&pass=37`), so a view can
be linked and the back button works; the old links (`#/experiments/7`, `#/garage`,
`#/propose?…`) still lead to the matching view.

**Two read-only routes in core** (`docs/api/openapi.yaml`, `contracts/session_passes` and
`contracts/pass_trace`), served by the new package `core/internal/passes`:

- `GET /sessions/{id}/passes`: every stored pass (corner, lap, zone time, validity, setup
  hash, block, conditions, the engine's flags and metrics) and per corner the best, median
  and sample standard deviation of the valid zone times. These are descriptive numbers of
  stored facts, computed in Go and tested; the web computes nothing but display units and
  colours.
- `GET /sessions/{id}/passes/{seq}/samples?max_points=`: the 24 channels of one pass from the
  session's Parquet parts under `PADDOCK_DATA_DIR/sessions/<spool id>/`, where ingest writes
  them. A part whose `seq` column statistics exclude the pass is not decoded. Above
  `max_points` (default 1000, 50 to 5000) every k-th sample is kept, and the last one;
  nothing is averaged or interpolated. A corner pass at 60 Hz has a few hundred samples, so
  thinning is rare.
- The samples reply also carries the slip angle per axle as the engine defines it
  (`engine/paddock_engine/features/axles.py`: load-weighted mean of the absolute slip angles),
  so the chart shows the same quantity the diagnosis talks about. The formula is duplicated in
  Go with the engine's cases as tests; if the engine's definition changes, both change.

No migration: the read path uses the tables and files that exist.

**uPlot for the charts** (`uplot` 1.6.32, MIT, pinned exactly). It draws on canvas, is about
50 kB minified, has no dependencies, and handles tens of thousands of points per chart
without effort. Its cursor sync moves one crosshair across the six stacked channel charts
(speed, throttle, brake, steering, front and rear slip); a readout table gives every overlaid
pass's values at the cursor, so the numbers are text, not only pixels. The wrapper
`web/src/components/charts/UPlotChart.tsx` is the only file that imports it.

Colours mean one thing everywhere: purple is the corner's best pass (as on a timing screen),
cyan the pass picked for a closer look.

## Alternatives considered

- **Recharts, Victory, Nivo (SVG and React).** Comfortable to write, but one SVG node per
  point: six charts of several 500-point traces become slow to draw and to hover, and the
  bundles are several times larger.
- **Chart.js.** Canvas and capable, but around four times larger than uPlot and not built for
  dense time series with a shared cursor.
- **Plotly.** Everything included, megabytes of it.
- **Hand-written SVG or canvas.** No dependency, but axes, ticks, scaling, hit-testing and
  cursor sync are exactly what a chart library is for.
- **Downsampling with min/max buckets (LTTB and the like).** Keeps peaks, but the kept
  samples differ per channel, so the channels of one pass would no longer share an x. Even
  thinning keeps them aligned and the reply says the step.
- **Serving samples through the engine (DuckDB).** The engine has no HTTP route for the web
  and should not become one; core already owns the files and the API.

## Consequences

- The web bundle grows by uPlot (about 50 kB, 20 kB gzipped).
- Charts cannot render in jsdom (no canvas): component tests replace `UPlotChart` with a
  stub that exposes its accessible name and data, and the data shaping
  (`telemetry/traceData.ts`) is tested on its own.
- Reading a pass opens Parquet parts until it finds the one that holds it; with one part per
  spool chunk that is a metadata read per part, measured at about 0.2 s per request on the
  first recorded session (102 passes, localhost, including the HTTP round trip).
- The read-only demo is unaffected: both routes are GETs, which its nginx lets through.

## Not verified

- Samples are read from `PADDOCK_DATA_DIR`, not from `session.parquet_uri`. They are the same
  path in every configuration that exists (Docker and the native run); a database moved to a
  machine with another data directory would need the files moved with it.
- Performance with sessions of thousands of passes was not measured.
