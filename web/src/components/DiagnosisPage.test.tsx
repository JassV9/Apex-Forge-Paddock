import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Diagnosis, Proposals } from "../api/client";
import { parseRoute } from "../hooks/useRoute";
import { diagnosis } from "../test-data";
import { type DiagnosisApi, DiagnosisPage } from "./DiagnosisPage";

function apiWith(change: Partial<DiagnosisApi> = {}): DiagnosisApi {
  return {
    load: () => Promise.resolve(diagnosis),
    run: vi.fn(() => Promise.resolve(diagnosis)),
    proposals: () => Promise.resolve(null),
    ...change,
  };
}

describe("DiagnosisPage", () => {
  it("shows a hypothesis with its evidence and the alternative the data speaks for", async () => {
    render(<DiagnosisPage sessionId={3} api={apiWith()} />);

    const card = await screen.findByRole("listitem", { name: "mid_understeer.slow" });

    expect(within(card).getByRole("heading", { name: /Understeer mid-corner/ })).toBeVisible();
    expect(within(card).getAllByText(/7\.67 over 32 passes/)[0]).toBeVisible();
    const driver = within(card).getByText("Driver input").closest("li") as HTMLElement;
    expect(within(driver).getByText("the data speaks for this")).toBeVisible();
    expect(within(driver).getByText(/front saturation/)).toBeVisible();
    expect(within(driver).getByText(/3\.71 over 5 passes/)).toBeVisible();
    const conditions = within(card).getByText("Conditions").closest("li") as HTMLElement;
    expect(within(conditions).queryByText("the data speaks for this")).toBeNull();
    expect(within(card).getByText(/left\/right pair/)).toBeInTheDocument();
  });

  it("hands a candidate to the propose form as an experiment, not as a fix", async () => {
    render(<DiagnosisPage sessionId={3} api={apiWith()} />);

    const link = await screen.findByRole("link", { name: "Test this" });

    expect(screen.getByText(/a rule of thumb, not a measurement/)).toBeVisible();
    expect(screen.getByText(/Front ARB −1 click:/)).toBeVisible();
    expect(parseRoute(link.getAttribute("href") ?? "")).toEqual({
      page: "propose",
      prefill: {
        sessionId: 3,
        parameter: "ARB_FRONT",
        clicks: -1,
        hypothesisKey: "mid_understeer.slow",
        statement: `${diagnosis.hypotheses[0]?.statement ?? ""} Test: a softer front bar gives more front grip mid-corner.`,
      },
    });
  });

  it("says who wrote the words, and why a model's reply was thrown away", async () => {
    const rejected: Diagnosis = {
      ...diagnosis,
      explanation: {
        ...diagnosis.explanation,
        rejected: "The reply was rejected: it states a number that is not in the pack: 0.35",
      },
    };
    render(
      <DiagnosisPage sessionId={3} api={apiWith({ load: () => Promise.resolve(rejected) })} />,
    );

    expect(await screen.findByText(/Written from templates/)).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("not in the pack: 0.35");
  });

  it("diagnoses a session that has no diagnosis yet", async () => {
    const api = apiWith({ load: () => Promise.resolve(null) });
    render(<DiagnosisPage sessionId={3} api={api} />);

    expect(await screen.findByText(/has not been diagnosed yet/)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Diagnose this session" }));

    expect(await screen.findByRole("listitem", { name: "mid_understeer.slow" })).toBeVisible();
    expect(api.run).toHaveBeenCalledWith(3);
    expect(screen.getByRole("button", { name: "Diagnose again" })).toBeEnabled();
  });

  it("ranks the experiments, and ranks them again after a new diagnosis", async () => {
    const ranking: Proposals = {
      stage: "cold_start",
      ranked: [],
      skipped: [{ parameter: "ARB_FRONT", label: "Front ARB", clicks: -1, reason: "Measured." }],
    };
    const proposals = vi.fn(() => Promise.resolve(ranking));
    render(<DiagnosisPage sessionId={3} api={apiWith({ proposals })} />);

    expect(await screen.findByRole("region", { name: "What to test next" })).toBeVisible();
    expect(proposals).toHaveBeenCalledExactlyOnceWith(3);
    fireEvent.click(screen.getByRole("button", { name: "Diagnose again" }));

    await vi.waitFor(() => {
      expect(proposals).toHaveBeenCalledTimes(2);
    });
  });

  it("shows the stored diagnosis in the read-only demo, without a way to run it", async () => {
    render(<DiagnosisPage sessionId={3} api={apiWith()} readOnly />);

    expect(await screen.findByRole("listitem", { name: "mid_understeer.slow" })).toBeVisible();
    expect(screen.queryByRole("button", { name: /Diagnose/ })).toBeNull();
  });

  it("says what core refused", async () => {
    const refusal = new Error("session 3 has no valid pass with computed features yet");
    const api = apiWith({ load: () => Promise.resolve(null), run: () => Promise.reject(refusal) });
    render(<DiagnosisPage sessionId={3} api={api} />);

    fireEvent.click(await screen.findByRole("button", { name: "Diagnose this session" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("no valid pass");
  });

  it("says so when no symptom was found", async () => {
    const clean: Diagnosis = {
      evidence: [],
      hypotheses: [],
      explanation: {
        provider: "none",
        summary: "No symptom is beyond its threshold.",
        ranking: [],
        rejected: "",
      },
    };
    render(<DiagnosisPage sessionId={3} api={apiWith({ load: () => Promise.resolve(clean) })} />);

    expect(await screen.findByText(/No symptom is beyond its threshold/)).toBeVisible();
    expect(screen.queryByRole("list")).toBeNull();
  });
});
