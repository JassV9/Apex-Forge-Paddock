import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "./App";

function pending(): Promise<Response> {
  return new Promise(() => undefined);
}

describe("App", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    window.location.hash = "";
  });

  it("offers every page when experiments can be run", () => {
    vi.stubGlobal("fetch", vi.fn(pending));

    render(<App readOnly={false} />);

    const pages = screen.getByRole("navigation", { name: "Pages" });
    expect(pages).toHaveTextContent("SessionsLedgerGaragePropose");
    expect(screen.queryByRole("note")).toBeNull();
  });

  it("says it is a read-only demo and offers nothing a visitor cannot do", () => {
    vi.stubGlobal("fetch", vi.fn(pending));
    window.location.hash = "#/propose";

    render(<App readOnly />);

    expect(screen.getByRole("note")).toHaveTextContent("Read-only demo on a recorded session");
    expect(screen.getByRole("navigation", { name: "Pages" })).toHaveTextContent(
      "SessionsLedgerGarage",
    );
    expect(screen.queryByRole("link", { name: "Propose" })).toBeNull();
    expect(screen.getByText(/cannot be proposed in the read-only demo/)).toBeVisible();
  });
});
