import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Experiment } from "../api/client";
import { running, tradeoff } from "../test-data";
import { type ExperimentApi, ExperimentPage } from "./ExperimentPage";

function apiFor(experiment: Experiment, change: Partial<ExperimentApi> = {}): ExperimentApi {
  return {
    load: vi.fn(() => Promise.resolve(experiment)),
    act: vi.fn(() => Promise.resolve(experiment)),
    decide: vi.fn(() => Promise.resolve(experiment)),
    ...change,
  };
}

describe("ExperimentPage", () => {
  it("shows every state transition in order, with its reason", async () => {
    render(<ExperimentPage id={4} api={apiFor(tradeoff)} />);

    const history = await screen.findByRole("region", { name: "History" });
    const entries = within(history).getAllByRole("listitem");

    expect(entries.map((entry) => entry.querySelector("strong")?.textContent)).toEqual([
      "Proposed",
      "Proposed → Planned",
      "Planned → Running",
      "Running → Evaluating",
      "Evaluating → Trade-off",
    ]);
    expect(entries[2]).toHaveTextContent("Run plan and setups written for the agent.");
    expect(entries[3]).toHaveTextContent("84 passes judged, 0 left out.");
  });

  it("does not say which setup is the baseline while the experiment is driven", async () => {
    render(<ExperimentPage id={4} api={apiFor(running)} />);

    const setups = await screen.findByRole("region", { name: "Setups" });

    expect(within(setups).getByText("Setup K")).toBeInTheDocument();
    expect(within(setups).getByText("Setup R")).toBeInTheDocument();
    expect(setups).not.toHaveTextContent(/: baseline|: variant|value/);
    expect(screen.queryByRole("region", { name: "Results" })).not.toBeInTheDocument();
    const plan = screen.getByRole("region", { name: "Run plan" });
    expect(within(plan).getByText("1 of 3")).toBeInTheDocument();
  });

  it("shows results, verdict and which setup was which once it is judged", async () => {
    render(<ExperimentPage id={4} api={apiFor(tradeoff)} />);

    const setups = await screen.findByRole("region", { name: "Setups" });

    expect(setups).toHaveTextContent("Setup K: baseline, value 3");
    expect(setups).toHaveTextContent("Setup R: variant, value 2");
    const results = screen.getByRole("region", { name: "Results" });
    expect(within(results).getByText("−0.045 [−0.060, −0.030]")).toBeInTheDocument();
    const verdict = screen.getByRole("region", { name: "Verdict" });
    expect(verdict).toHaveTextContent("decided by the engine");
    expect(verdict).toHaveTextContent("Predictions that came true: 1 of 1.");
  });

  it("sends the driver's call on a trade-off and shows the experiment that comes back", async () => {
    const accepted: Experiment = { ...tradeoff, state: "accepted" };
    const api = apiFor(tradeoff, { decide: vi.fn(() => Promise.resolve(accepted)) });
    render(<ExperimentPage id={4} api={api} />);

    fireEvent.click(await screen.findByRole("button", { name: "Keep the variant" }));

    expect(await screen.findByRole("button", { name: "Roll back" })).toBeInTheDocument();
    expect(api.decide).toHaveBeenCalledWith(4, true);
    expect(api.act).not.toHaveBeenCalled();
  });

  it("says what core refused and keeps the experiment on screen", async () => {
    const refusal = new Error("the features of 12 passes are not computed yet");
    const api = apiFor(running, { act: vi.fn(() => Promise.reject(refusal)) });
    render(<ExperimentPage id={4} api={api} />);

    fireEvent.click(await screen.findByRole("button", { name: "Evaluate" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("12 passes are not computed yet");
    expect(api.act).toHaveBeenCalledWith(4, "evaluate");
    expect(screen.getByRole("region", { name: "History" })).toBeInTheDocument();
  });

  it("reports an experiment that cannot be loaded", async () => {
    const api = apiFor(running, { load: () => Promise.reject(new Error("no such experiment")) });
    render(<ExperimentPage id={99} api={api} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("no such experiment");
  });
});
