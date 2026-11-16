import { useEffect, useState } from "react";

/** What a diagnosis candidate hands to the propose form. */
export interface Prefill {
  sessionId: number;
  parameter: string;
  clicks: number;
  statement: string;
  /** Key of the diagnosis hypothesis the experiment tests. */
  hypothesisKey: string;
  /** Gain per corner pass in seconds the car's profile expects, when it expects one. */
  gain?: { low: number; high: number };
}

export const TABS = [
  "overview",
  "telemetry",
  "reference",
  "diagnosis",
  "experiments",
  "garage",
] as const;
export type Tab = (typeof TABS)[number];

/**
 * Where the workspace is: a session (the newest when absent), one of its tabs, and what is
 * open in that tab. Everything is in the location hash, so a view can be linked and the
 * browser's back button works.
 */
export interface Route {
  sessionId?: number;
  tab: Tab;
  /** Telemetry: the corner and the pass picked. */
  corner?: number;
  pass?: number;
  /** Experiments: the experiment opened inline. */
  experimentId?: number;
  /** Experiments: the propose panel is open, blank or filled from a candidate. */
  propose?: Prefill | "blank";
}

function parseGain(params: URLSearchParams): Prefill["gain"] {
  const low = params.get("low");
  const high = params.get("high");
  if (low === null || high === null) return undefined;
  const gain = { low: Number(low), high: Number(high) };
  return gain.low >= 0 && gain.high >= gain.low ? gain : undefined;
}

function parsePrefill(params: URLSearchParams, sessionId?: number): Prefill | undefined {
  const session = sessionId ?? Number(params.get("session"));
  const clicks = Number(params.get("clicks"));
  const parameter = params.get("parameter") ?? "";
  if (!Number.isInteger(session) || session < 1 || parameter === "") return undefined;
  if (!Number.isInteger(clicks) || clicks === 0) return undefined;
  const gain = parseGain(params);
  return {
    sessionId: session,
    parameter,
    clicks,
    statement: params.get("statement") ?? "",
    hypothesisKey: params.get("hypothesis") ?? "",
    ...(gain === undefined ? {} : { gain }),
  };
}

function positive(text: string | null | undefined): number | undefined {
  if (text === null || text === undefined || !/^[1-9]\d*$/.test(text)) return undefined;
  return Number(text);
}

function withPicks(route: Route, params: URLSearchParams): Route {
  const corner = positive(params.get("corner"));
  const pass = positive(params.get("pass"));
  return {
    ...route,
    ...(corner === undefined ? {} : { corner }),
    ...(pass === undefined ? {} : { pass }),
  };
}

/** A view inside one session: /sessions/{id}[/{tab}[/{experiment}]] or /sessions/{id}/propose. */
function parseSessionPath(id: number, rest: string[], params: URLSearchParams): Route {
  const [view = "overview", detail] = rest;
  if (view === "propose") {
    return { sessionId: id, tab: "experiments", propose: parsePrefill(params, id) ?? "blank" };
  }
  const tab = (TABS as readonly string[]).includes(view) ? (view as Tab) : "overview";
  const experimentId = tab === "experiments" ? positive(detail) : undefined;
  const route: Route = {
    sessionId: id,
    tab,
    ...(experimentId === undefined ? {} : { experimentId }),
  };
  return tab === "telemetry" ? withPicks(route, params) : route;
}

/** Links of the earlier pages (#/experiments/7, #/garage, #/propose?…) still lead somewhere. */
function parseLegacyPath(parts: string[], params: URLSearchParams): Route {
  const [page, detail] = parts;
  if (page === "experiments") {
    const experimentId = positive(detail);
    return { tab: "experiments", ...(experimentId === undefined ? {} : { experimentId }) };
  }
  if (page === "garage") return { tab: "garage" };
  if (page === "propose") {
    const prefill = parsePrefill(params);
    if (prefill === undefined) return { tab: "experiments", propose: "blank" };
    return { sessionId: prefill.sessionId, tab: "experiments", propose: prefill };
  }
  return { tab: "overview" };
}

/** Which view a location hash names. Anything unknown is the newest session's overview. */
export function parseRoute(hash: string): Route {
  const [path = "", query = ""] = hash.replace(/^#\/?/, "").split("?", 2);
  const params = new URLSearchParams(query);
  const parts = path.split("/").filter((part) => part !== "");
  if (parts[0] === "sessions") {
    const id = positive(parts[1]);
    if (id !== undefined) return parseSessionPath(id, parts.slice(2), params);
  }
  return parseLegacyPath(parts, params);
}

export function sessionHref(id: number, tab: Tab = "overview"): string {
  return tab === "overview" ? `#/sessions/${String(id)}` : `#/sessions/${String(id)}/${tab}`;
}

/** The telemetry tab on one corner, optionally with one pass picked. */
export function telemetryHref(sessionId: number, corner: number, pass?: number): string {
  const params = new URLSearchParams({ corner: String(corner) });
  if (pass !== undefined) params.set("pass", String(pass));
  return `${sessionHref(sessionId, "telemetry")}?${params.toString()}`;
}

export function experimentHref(sessionId: number, id: number): string {
  return `${sessionHref(sessionId, "experiments")}/${String(id)}`;
}

/** The propose panel, blank or filled in with one candidate experiment of a diagnosis. */
export function proposeHref(sessionId: number, prefill?: Prefill): string {
  const base = `#/sessions/${String(sessionId)}/propose`;
  if (prefill === undefined) return base;
  const params = new URLSearchParams({
    parameter: prefill.parameter,
    clicks: String(prefill.clicks),
    hypothesis: prefill.hypothesisKey,
    statement: prefill.statement,
  });
  if (prefill.gain !== undefined) {
    params.set("low", String(prefill.gain.low));
    params.set("high", String(prefill.gain.high));
  }
  return `${base}?${params.toString()}`;
}

/** The view named by the location hash; follows the browser's back and forward. */
export function useRoute(): Route {
  const [route, setRoute] = useState(() => parseRoute(window.location.hash));

  useEffect(() => {
    const onChange = () => {
      setRoute(parseRoute(window.location.hash));
    };
    window.addEventListener("hashchange", onChange);
    return () => {
      window.removeEventListener("hashchange", onChange);
    };
  }, []);

  return route;
}
