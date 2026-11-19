import createClient from "openapi-fetch";

import type { components, paths } from "./schema";

export type Health = components["schemas"]["Health"];
export type SessionSummary = components["schemas"]["SessionSummary"];
export type SetupItem = components["schemas"]["SetupItem"];
export type Diagnosis = components["schemas"]["Diagnosis"];
export type Hypothesis = Diagnosis["hypotheses"][number];
export type Evidence = Diagnosis["evidence"][number];
export type SensitivityProfile = components["schemas"]["SensitivityProfile"];
export type SensitivityEffect = SensitivityProfile["effects"][number];
export type Proposals = components["schemas"]["Proposals"];
export type Proposal = Proposals["ranked"][number];
export type Scorecard = components["schemas"]["Scorecard"];
export type Experiment = components["schemas"]["Experiment"];
export type ExperimentCreate = components["schemas"]["ExperimentCreate"];
export type ExperimentResult = components["schemas"]["ExperimentResult"];
export type SessionPasses = components["schemas"]["SessionPasses"];
export type PassRecord = SessionPasses["passes"][number];
export type CornerSummary = SessionPasses["corners"][number];
export type PassTrace = components["schemas"]["PassTrace"];
export type TrainingProgram = components["schemas"]["TrainingProgram"];
export type Sweep = components["schemas"]["Sweep"];
export type SweepCreate = components["schemas"]["SweepCreate"];
export type CurveResult = NonNullable<Sweep["curve"]>["curves"][number];
export type SessionReference = components["schemas"]["SessionReference"];
export type CornerReference = SessionReference["corners"][number];
export type FeatureDifference = CornerReference["differences"][number];
export type ExperimentAction =
  paths["/experiments/{id}/{action}"]["post"]["parameters"]["path"]["action"];

/** Typed client for core's REST API; every path and body comes from docs/api/openapi.yaml. */
export const coreApi = createClient<paths>({ baseUrl: "/api" });

/** The body of a reply, or an error that says what core refused (its Problem body). */
export function unwrap<T>(reply: { data?: T; error?: unknown }, request: string): T {
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

/** The session's candidate experiments, ranked; null when it was never diagnosed. */
export async function fetchProposals(sessionId: number): Promise<Proposals | null> {
  const reply = await coreApi.GET("/sessions/{id}/proposals", {
    params: { path: { id: sessionId } },
  });
  if (reply.response.status === 404) return null;
  return unwrap(reply, "GET /sessions/{id}/proposals");
}

/** Every corner pass of a session, with the zone times per corner. */
export async function fetchPasses(sessionId: number): Promise<SessionPasses> {
  const reply = await coreApi.GET("/sessions/{id}/passes", { params: { path: { id: sessionId } } });
  return unwrap(reply, "GET /sessions/{id}/passes");
}

/** The raw samples of one pass, at most maxPoints per channel. */
export async function fetchTrace(
  sessionId: number,
  seq: number,
  maxPoints?: number,
): Promise<PassTrace> {
  const reply = await coreApi.GET("/sessions/{id}/passes/{seq}/samples", {
    params: {
      path: { id: sessionId, seq },
      query: maxPoints === undefined ? {} : { max_points: maxPoints },
    },
  });
  return unwrap(reply, "GET /sessions/{id}/passes/{seq}/samples");
}

/** The session's corners against AC's AI on the same car and setup (docs/adr/0019). */
export async function fetchReference(
  sessionId: number,
  setupHash?: string,
): Promise<SessionReference> {
  const reply = await coreApi.GET("/sessions/{id}/reference", {
    params: {
      path: { id: sessionId },
      query: setupHash === undefined ? {} : { setup_hash: setupHash },
    },
  });
  return unwrap(reply, "GET /sessions/{id}/reference");
}

/** Every car's sensitivity profile, rebuilt from the finished experiments. */
export async function fetchSensitivity(): Promise<SensitivityProfile[]> {
  return unwrap(await coreApi.GET("/sensitivity"), "GET /sensitivity");
}

export async function fetchScorecard(): Promise<Scorecard> {
  return unwrap(await coreApi.GET("/scorecard"), "GET /scorecard");
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

/** Every AI training program, newest first. */
export async function fetchTraining(): Promise<TrainingProgram[]> {
  return unwrap(await coreApi.GET("/training"), "GET /training");
}

/** Starts an AI training program on the session's car (docs/adr/0020-ai-training-program.md). */
export async function startTraining(
  sessionId: number,
  experiments: number,
): Promise<TrainingProgram> {
  const reply = await coreApi.POST("/sessions/{id}/training", {
    params: { path: { id: sessionId } },
    body: { experiments },
  });
  return unwrap(reply, "POST /sessions/{id}/training");
}

export async function stopTraining(id: number): Promise<TrainingProgram> {
  const reply = await coreApi.POST("/training/{id}/stop", { params: { path: { id } } });
  return unwrap(reply, "POST /training/{id}/stop");
}

/** Every sensitivity sweep, newest first (docs/adr/0032-sensitivity-discovery-sweeps.md). */
export async function fetchSweeps(): Promise<Sweep[]> {
  return unwrap(await coreApi.GET("/sweeps"), "GET /sweeps");
}

/** Starts a sweep of one setting on the session's car at a pinned AI hint. */
export async function startSweep(sessionId: number, body: SweepCreate): Promise<Sweep> {
  const reply = await coreApi.POST("/sessions/{id}/sweeps", {
    params: { path: { id: sessionId } },
    body,
  });
  return unwrap(reply, "POST /sessions/{id}/sweeps");
}

export async function stopSweep(id: number): Promise<Sweep> {
  const reply = await coreApi.POST("/sweeps/{id}/stop", { params: { path: { id } } });
  return unwrap(reply, "POST /sweeps/{id}/stop");
}

/** The driver's call on a trade-off: keep the variant, or go back to the baseline. */
export async function decideExperiment(id: number, accept: boolean): Promise<Experiment> {
  const reply = await coreApi.POST("/experiments/{id}/decision", {
    params: { path: { id } },
    body: { accept },
  });
  return unwrap(reply, "POST /experiments/{id}/decision");
}
