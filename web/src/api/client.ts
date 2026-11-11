import createClient from "openapi-fetch";

import type { components, paths } from "./schema";

export type Health = components["schemas"]["Health"];
export type SessionSummary = components["schemas"]["SessionSummary"];

/** Typed client for core's REST API; every path and body comes from docs/api/openapi.yaml. */
const coreApi = createClient<paths>({ baseUrl: "/api" });

export async function fetchCoreHealth(): Promise<Health> {
  const { data } = await coreApi.GET("/health");
  if (data === undefined) {
    throw new Error("core did not answer GET /health");
  }
  return data;
}

export async function fetchSessions(): Promise<SessionSummary[]> {
  const { data } = await coreApi.GET("/sessions");
  if (data === undefined) {
    throw new Error("core did not answer GET /sessions");
  }
  return data;
}
