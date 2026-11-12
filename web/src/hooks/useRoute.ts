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

export type Route =
  | { page: "sessions" }
  | { page: "session"; id: number }
  | { page: "ledger" }
  | { page: "garage" }
  | { page: "propose"; prefill?: Prefill }
  | { page: "experiment"; id: number };

function parseGain(params: URLSearchParams): Prefill["gain"] {
  const low = params.get("low");
  const high = params.get("high");
  if (low === null || high === null) return undefined;
  const gain = { low: Number(low), high: Number(high) };
  return gain.low >= 0 && gain.high >= gain.low ? gain : undefined;
}

function parsePrefill(query: string): Prefill | undefined {
  const params = new URLSearchParams(query);
  const sessionId = Number(params.get("session"));
  const clicks = Number(params.get("clicks"));
  const parameter = params.get("parameter") ?? "";
  if (!Number.isInteger(sessionId) || sessionId < 1 || parameter === "") return undefined;
  if (!Number.isInteger(clicks) || clicks === 0) return undefined;
  const gain = parseGain(params);
  return {
    sessionId,
    parameter,
    clicks,
    statement: params.get("statement") ?? "",
    hypothesisKey: params.get("hypothesis") ?? "",
    ...(gain === undefined ? {} : { gain }),
  };
}

/** Which page a location hash names. Anything unknown is the sessions page. */
export function parseRoute(hash: string): Route {
  const [path = "", query = ""] = hash.replace(/^#\/?/, "").split("?", 2);
  const experiment = /^experiments\/([1-9]\d*)$/.exec(path);
  if (experiment?.[1] !== undefined) return { page: "experiment", id: Number(experiment[1]) };
  const session = /^sessions\/([1-9]\d*)$/.exec(path);
  if (session?.[1] !== undefined) return { page: "session", id: Number(session[1]) };
  if (path === "experiments") return { page: "ledger" };
  if (path === "garage") return { page: "garage" };
  if (path === "propose") {
    const prefill = parsePrefill(query);
    return prefill === undefined ? { page: "propose" } : { page: "propose", prefill };
  }
  return { page: "sessions" };
}

export function experimentHref(id: number): string {
  return `#/experiments/${String(id)}`;
}

export function sessionHref(id: number): string {
  return `#/sessions/${String(id)}`;
}

/** The propose form, filled in with one candidate experiment of a diagnosis. */
export function proposeHref(prefill: Prefill): string {
  const params = new URLSearchParams({
    session: String(prefill.sessionId),
    parameter: prefill.parameter,
    clicks: String(prefill.clicks),
    hypothesis: prefill.hypothesisKey,
    statement: prefill.statement,
  });
  if (prefill.gain !== undefined) {
    params.set("low", String(prefill.gain.low));
    params.set("high", String(prefill.gain.high));
  }
  return `#/propose?${params.toString()}`;
}

/** The page named by the location hash; follows the browser's back and forward. */
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
