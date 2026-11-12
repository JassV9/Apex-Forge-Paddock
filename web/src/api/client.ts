import createClient from "openapi-fetch";

import type { components, paths } from "./schema";

export type Health = components["schemas"]["Health"];
export type SessionSummary = components["schemas"]["SessionSummary"];
export type SetupItem = components["schemas"]["SetupItem"];
export type Diagnosis = components["schemas"]["Diagnosis"];
export type Hypothesis = Diagnosis["hypotheses"][number];
export type Evidence = Diagnosis["evidence"][number];
export type Experiment = components["schemas"]["Experiment"];
export type ExperimentCreate = components["schemas"]["ExperimentCreate"];
export type ExperimentResult = components["schemas"]["ExperimentResult"];
export type ExperimentAction =
  paths["/experiments/{id}/{action}"]["post"]["parameters"]["path"]["action"];

/** Typed client for core's REST API; every path and body comes from docs/api/openapi.yaml. */
const coreApi = createClient<paths>({ baseUrl: "/api" });

/** The body of a reply, or an error that says what core refused (its Problem body). */
function unwrap<T>(reply: { data?: T; error?: unknown }, request: string): T {
  if (reply.data !== undefined) return reply.data;
  const problem = reply.error as { error?: unknown } | undefined;
  const said = typeof problem?.error === "string" ? problem.error : "";
  throw new Error(said === "" ? `core did not answer ${request}` : said);
}

export async function fetchCoreHealth(): Promise<Health> {
  return unwrap(await coreApi.GET("/health"), "GET /health");
}

export async function fetchSessions(): Promise<SessionSummary[]> {
  return unwrap(await coreApi.GET("/sessions"), "GET /sessions");
}

export async function fetchSetupItems(sessionId: number): Promise<SetupItem[]> {
  const reply = await coreApi.GET("/sessions/{id}/setup", { params: { path: { id: sessionId } } });
  return unwrap(reply, "GET /sessions/{id}/setup");
}

/** The newest stored diagnosis of a session; null when it was never diagnosed. */
export async function fetchDiagnosis(sessionId: number): Promise<Diagnosis | null> {
  const reply = await coreApi.GET("/sessions/{id}/diagnosis", {
    params: { path: { id: sessionId } },
  });
  if (reply.response.status === 404) return null;
  return unwrap(reply, "GET /sessions/{id}/diagnosis");
}

/** Diagnoses the session now; core keeps the answer. */
export async function runDiagnosis(sessionId: number): Promise<Diagnosis> {
  const reply = await coreApi.POST("/sessions/{id}/diagnosis", {
    params: { path: { id: sessionId } },
  });
  return unwrap(reply, "POST /sessions/{id}/diagnosis");
}

export async function fetchExperiments(): Promise<Experiment[]> {
  return unwrap(await coreApi.GET("/experiments"), "GET /experiments");
}

export async function fetchExperiment(id: number): Promise<Experiment> {
  const reply = await coreApi.GET("/experiments/{id}", { params: { path: { id } } });
  return unwrap(reply, "GET /experiments/{id}");
}

export async function createExperiment(body: ExperimentCreate): Promise<Experiment> {
  return unwrap(await coreApi.POST("/experiments", { body }), "POST /experiments");
}

/** Moves an experiment along its state machine; core refuses what its state does not allow. */
export async function actOnExperiment(id: number, action: ExperimentAction): Promise<Experiment> {
  const reply = await coreApi.POST("/experiments/{id}/{action}", {
    params: { path: { id, action } },
  });
  return unwrap(reply, `POST /experiments/{id}/${action}`);
}

/** The driver's call on a trade-off: keep the variant, or go back to the baseline. */
export async function decideExperiment(id: number, accept: boolean): Promise<Experiment> {
  const reply = await coreApi.POST("/experiments/{id}/decision", {
    params: { path: { id } },
    body: { accept },
  });
  return unwrap(reply, "POST /experiments/{id}/decision");
}
