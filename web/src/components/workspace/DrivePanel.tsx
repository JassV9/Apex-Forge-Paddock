import { useState } from "react";

import type { LauncherContent, LauncherStatus } from "../../api/launcher";
import {
  blockedReason,
  findLayout,
  layoutHint,
  layoutKey,
  layoutLabel,
  openWorkNotes,
  parseLayoutKey,
  START_LAUNCHER,
  START_LAUNCHER_BY_HAND,
  statusLines,
} from "./driveText";
import { type LauncherApi, launcherClient, useLauncher } from "./useLauncher";
import { coreOpenWork, type OpenWorkApi, useOpenWork } from "./useOpenWork";

/** How often core is asked what is open on the car. */
const OPEN_WORK_MS = 15_000;

interface DrivePanelProps {
  api?: LauncherApi;
  /** What core says is open on the car; the real client by default. */
  openWork?: OpenWorkApi;
  pollMs?: number;
}

function Unreachable() {
  return (
    <div className="drive-help" role="note">
      <p>
        The launcher is not running. It starts the game on this PC, outside Docker. In the repo:
      </p>
      <code>{START_LAUNCHER}</code>
      <p>
        or <code>{START_LAUNCHER_BY_HAND}</code>, then leave that window open.
      </p>
    </div>
  );
}

function Pickers({
  content,
  car,
  layout,
  onCar,
  onLayout,
}: {
  content: LauncherContent;
  car: string;
  layout: string;
  onCar: (id: string) => void;
  onLayout: (key: string) => void;
}) {
  return (
    <>
      <label>
        Car
        <select
          value={car}
          onChange={(e) => {
            onCar(e.target.value);
          }}
        >
          <option value="">Pick a car</option>
          {content.cars.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
      </label>
      <label>
        Track
        <select
          value={layout}
          onChange={(e) => {
            onLayout(e.target.value);
          }}
        >
          <option value="">Pick a track</option>
          {content.tracks.map((track) => (
            <optgroup key={track.id} label={track.name}>
              {track.layouts.map((l) => (
                <option key={l.layout} value={layoutKey(track.id, l.layout)}>
                  {layoutLabel(l)}
                </option>
              ))}
            </optgroup>
          ))}
        </select>
      </label>
    </>
  );
}

function Hint({
  content,
  layout,
  onLayout,
}: {
  content: LauncherContent;
  layout: string;
  onLayout: (key: string) => void;
}) {
  const found = findLayout(content, layout);
  if (found === undefined) return null;
  const hint = layoutHint(found.track, found.layout);
  return (
    <>
      <p className={hint.aiCanDrive ? "drive-hint is-good" : "drive-hint is-warn"}>
        {hint.text} {hint.command !== undefined && <code>{hint.command}</code>}
      </p>
      {hint.testCopy !== undefined && (
        <button
          type="button"
          className="quiet"
          onClick={() => {
            onLayout(layoutKey(found.track.id, hint.testCopy ?? ""));
          }}
        >
          Use the test copy
        </button>
      )}
    </>
  );
}

/** What is open on the picked car, so that starting the game holds no surprise. */
function OpenWorkNotice({ notes }: { notes: string[] }) {
  if (notes.length === 0) return null;
  return (
    <div className="drive-help" role="note" aria-label="Open on this car">
      {notes.map((note) => (
        <p key={note}>{note}</p>
      ))}
    </div>
  );
}

/** The last launch's car and layout, so the pickers start where the owner left off. */
function lastPicks(status: LauncherStatus | undefined): { car: string; layout: string } {
  const last = status?.last_launch;
  if (last === undefined) return { car: "", layout: "" };
  return { car: last.car_id, layout: layoutKey(last.track_id, last.layout) };
}

function Launcher({
  api,
  openWork,
  content,
  status,
  onStatus,
}: {
  api: LauncherApi;
  openWork: OpenWorkApi;
  content: LauncherContent | undefined;
  status: LauncherStatus;
  onStatus: (status: LauncherStatus) => void;
}) {
  const [picks, setPicks] = useState(() => lastPicks(status));
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const carName = (id: string) => content?.cars.find((c) => c.id === id)?.name ?? id;
  const blocked = blockedReason(status);
  const work = useOpenWork(openWork, OPEN_WORK_MS);
  const ready = picks.car !== "" && picks.layout !== "" && blocked === undefined && !sending;
  const start = () => {
    const { trackId, layout } = parseLayoutKey(picks.layout);
    setSending(true);
    setError("");
    api.launch({ car_id: picks.car, track_id: trackId, layout }).then(
      (next) => {
        setSending(false);
        onStatus(next);
      },
      (reason: unknown) => {
        setSending(false);
        setError(reason instanceof Error ? reason.message : "The launcher refused.");
      },
    );
  };
  return (
    <>
      {content === undefined ? (
        <p className="drive-hint">Reading the installed cars and tracks…</p>
      ) : (
        <>
          <Pickers
            content={content}
            car={picks.car}
            layout={picks.layout}
            onCar={(car) => {
              setPicks({ ...picks, car });
            }}
            onLayout={(layout) => {
              setPicks({ ...picks, layout });
            }}
          />
          <Hint
            content={content}
            layout={picks.layout}
            onLayout={(layout) => {
              setPicks({ ...picks, layout });
            }}
          />
          <OpenWorkNotice
            notes={openWorkNotes(work.experiments, work.programs, carName(picks.car))}
          />
        </>
      )}
      <button type="button" disabled={!ready} onClick={start}>
        {sending ? "Starting…" : "Start driving"}
      </button>
      <p className="drive-hint">
        You drive. The game opens in Practice from the pits with your own Content Manager settings,
        clear weather and a fully gripped track; the Paddock agent only records your laps.
      </p>
      {blocked !== undefined && <p className="drive-hint">{blocked}</p>}
      {error !== "" && <p role="alert">{error}</p>}
      <ul className="drive-status" aria-label="Launcher status">
        {statusLines(status, carName).map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ul>
    </>
  );
}

/** Pick a car and a layout and start Assetto Corsa in Practice from the pits, through the
 * host launcher and Content Manager. The panel shows what the launcher says; it decides nothing. */
export function DrivePanel({
  api = launcherClient,
  openWork = coreOpenWork,
  pollMs = 3000,
}: DrivePanelProps) {
  const { reach, content, showStatus } = useLauncher(api, pollMs);
  return (
    // The rail closes on a click on small screens; the pickers must not close it.
    <section
      className="drive"
      aria-label="Drive"
      onClick={(e) => {
        e.stopPropagation();
      }}
    >
      <h2 className="rail-day">Drive</h2>
      {reach.kind === "looking" && <p className="drive-hint">Looking for the launcher…</p>}
      {reach.kind === "unreachable" && <Unreachable />}
      {reach.kind === "ready" && (
        <Launcher
          api={api}
          openWork={openWork}
          content={content}
          status={reach.status}
          onStatus={showStatus}
        />
      )}
    </section>
  );
}
