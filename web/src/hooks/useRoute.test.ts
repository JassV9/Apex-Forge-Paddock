import { describe, expect, it } from "vitest";

import { experimentHref, parseRoute, type Route } from "./useRoute";

describe("parseRoute", () => {
  const cases: [string, Route][] = [
    ["", { page: "sessions" }],
    ["#/", { page: "sessions" }],
    ["#/experiments", { page: "ledger" }],
    ["#/propose", { page: "propose" }],
    ["#/experiments/12", { page: "experiment", id: 12 }],
    ["#/experiments/0", { page: "sessions" }],
    ["#/experiments/abc", { page: "sessions" }],
    ["#/somewhere-else", { page: "sessions" }],
  ];

  it.each(cases)("reads %j", (hash, route) => {
    expect(parseRoute(hash)).toEqual(route);
  });

  it("reads back the link of an experiment", () => {
    expect(parseRoute(experimentHref(7))).toEqual({ page: "experiment", id: 7 });
  });
});
