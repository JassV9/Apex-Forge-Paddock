import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Scorecard as Card } from "../api/client";
import { Scorecard } from "./Scorecard";

const card: Card = {
  experiments: 9,
  judged: 7,
  predictions_total: 8,
  predictions_hit: 3,
  by_source: [
    { source: "human", experiments: 5, predictions_total: 4, predictions_hit: 1 },
    { source: "rules", experiments: 4, predictions_total: 4, predictions_hit: 2 },
    { source: "llm", experiments: 0, predictions_total: 0, predictions_hit: 0 },
  ],
  by_state: [{ state: "accepted", count: 2 }],
};

describe("Scorecard", () => {
  it("counts the predictions that came true, overall and by where the hypothesis came from", async () => {
    render(<Scorecard loadScorecard={() => Promise.resolve(card)} />);

    const section = await screen.findByRole("region", { name: "Scorecard" });

    expect(within(section).getByText("3 of 8 (38 %)")).toBeVisible();
    expect(section).toHaveTextContent("over 7 judged of 9 experiments");
    const rules = within(section).getByText("Diagnosis rules").closest("tr") as HTMLElement;
    expect(within(rules).getByText("2 of 4 (50 %)")).toBeVisible();
    const model = within(section).getByText("Language model").closest("tr") as HTMLElement;
    expect(within(model).getByText("none scored yet")).toBeVisible();
  });

  it("stays out of the way on an empty ledger", async () => {
    const empty: Card = { ...card, experiments: 0, judged: 0, by_source: [], by_state: [] };
    const { container } = render(<Scorecard loadScorecard={() => Promise.resolve(empty)} />);

    await Promise.resolve();

    expect(container).toBeEmptyDOMElement();
  });

  it("reports a failed load", async () => {
    render(<Scorecard loadScorecard={() => Promise.reject(new Error("500"))} />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The scorecard could not be loaded.",
    );
  });
});
