import { useEffect, useState } from "react";

export type Route =
  | { page: "sessions" }
  | { page: "ledger" }
  | { page: "propose" }
  | { page: "experiment"; id: number };

/** Which page a location hash names. Anything unknown is the sessions page. */
export function parseRoute(hash: string): Route {
  const path = hash.replace(/^#\/?/, "");
  const experiment = /^experiments\/([1-9]\d*)$/.exec(path);
  if (experiment?.[1] !== undefined) return { page: "experiment", id: Number(experiment[1]) };
  if (path === "experiments") return { page: "ledger" };
  if (path === "propose") return { page: "propose" };
  return { page: "sessions" };
}

export function experimentHref(id: number): string {
  return `#/experiments/${String(id)}`;
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
