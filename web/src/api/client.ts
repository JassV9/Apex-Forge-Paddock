import createClient from "openapi-fetch";

import type { components, paths } from "./schema";

export type Health = components["schemas"]["Health"];

/** Typed client for core's REST API; every path and body comes from docs/api/openapi.yaml. */
const coreApi = createClient<paths>({ baseUrl: "/api" });

export async function fetchCoreHealth(): Promise<Health> {
  const { data } = await coreApi.GET("/health");
  if (data === undefined) {
    throw new Error("core did not answer GET /health");
  }
  return data;
}
