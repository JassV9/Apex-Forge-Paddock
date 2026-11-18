import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { cotaReference } from "../../test-reference";
import { ReferencePanel } from "./ReferencePanel";

describe("ReferencePanel", () => {
  it("lists the corners by time lost with the AI, the verdict and what differs", async () => {
    render(<ReferencePanel sessionId={2} load={() => Promise.resolve(cotaReference)} />);

    const table = await screen.findByRole("table", { name: /time lost to the AI/ });
    const rows = within(table).getAllByRole("row");

    expect(rows).toHaveLength(3);
    expect(rows[1]).toHaveTextContent("T19.1649.2009.050 ±0.003+0.150Practise");
    expect(rows[1]).toHaveTextContent("Brake point (m before apex): 120 vs AI 100 (2 / 9 passes)");
    expect(rows[2]).toHaveTextContent("Car / setup");
    expect(rows[2]).toHaveTextContent("AI on the nearest setup: 2 items differ");
    expect(within(rows[1] as HTMLElement).getByRole("link", { name: "T1" })).toHaveAttribute(
      "href",
      "#/sessions/2/telemetry?corner=1",
    );
    expect(screen.getByText(/40 AI passes on this car and track/)).toBeVisible();
  });

  it("compares the original layout with the AI on its test copy, labelled as the same track", async () => {
    const original = {
      ...cotaReference,
      layout: "layout_f1_2026",
      ai_tracks: [
        { layout: "layout_f1_2026", name: "COTA F1 2026", variant: false, ai_passes: 0 },
        {
          layout: "layout_f1_2026_paddock",
          name: "COTA F1 2026 (Paddock test)",
          variant: true,
          ai_passes: 40,
        },
      ],
    };
    render(<ReferencePanel sessionId={2} load={() => Promise.resolve(original)} />);

    expect(
      await screen.findByText(/AI on COTA F1 2026 \(Paddock test\), the same track/),
    ).toHaveTextContent("Compared in the web app only");
    expect(screen.getByText(/40 AI passes on this car and track/)).toBeVisible();
    expect(screen.queryByRole("note")).not.toBeInTheDocument();
  });

  it("says plainly why there is nothing to compare with on the original layout yet", async () => {
    const original = { ...cotaReference, layout: "layout_f1_2026", ai_passes: 0 };
    render(<ReferencePanel sessionId={2} load={() => Promise.resolve(original)} />);

    const note = await screen.findByRole("note");
    expect(note).toHaveTextContent("You drove the original layout (layout_f1_2026)");
    expect(note).toHaveTextContent("compared here, in the web app");
    expect(note).toHaveTextContent("never shown in the game");
    expect(note).not.toHaveTextContent("not compared with the AI");
    expect(screen.queryByText(/AI passes on this car and track/)).not.toBeInTheDocument();
  });

  it("says why a test copy with another corner map is not compared with", async () => {
    const differs = {
      ...cotaReference,
      layout: "layout_f1_2026",
      ai_passes: 0,
      variant_note:
        "The AI's laps on COTA F1 2026 (Paddock test) are not compared with: corner 2's apex is 50.0 m from this layout's, more than 5 m.",
    };
    render(<ReferencePanel sessionId={2} load={() => Promise.resolve(differs)} />);

    expect(await screen.findByRole("note")).toHaveTextContent("corner 2's apex is 50.0 m");
  });

  it("says the AI has not driven the test copy yet, when it has not", async () => {
    const unseen = { ...cotaReference, ai_passes: 0 };
    render(<ReferencePanel sessionId={2} load={() => Promise.resolve(unseen)} />);

    expect(await screen.findByRole("note")).toHaveTextContent(
      "The AI has not driven this car on this layout yet",
    );
  });

  it("compares another setup of the session when it is picked", async () => {
    const load = vi.fn(() => Promise.resolve(cotaReference));
    render(<ReferencePanel sessionId={2} load={load} />);

    fireEvent.change(await screen.findByRole("combobox"), { target: { value: "s2" } });

    await vi.waitFor(() => {
      expect(load).toHaveBeenLastCalledWith(2, "s2");
    });
    expect(load).toHaveBeenNthCalledWith(1, 2, undefined);
  });

  it("says when the comparison could not be loaded", async () => {
    render(<ReferencePanel sessionId={2} load={() => Promise.reject(new Error("500"))} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("could not be loaded");
  });
});
