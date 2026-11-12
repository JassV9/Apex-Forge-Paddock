import { describe, expect, it } from "vitest";

import { experimentHref, parseRoute, proposeHref, type Route, sessionHref } from "./useRoute";

describe("parseRoute", () => {
  const cases: [string, Route][] = [
    ["", { page: "sessions" }],
    ["#/", { page: "sessions" }],
    ["#/experiments", { page: "ledger" }],
    ["#/garage", { page: "garage" }],
    ["#/propose", { page: "propose" }],
    ["#/experiments/12", { page: "experiment", id: 12 }],
    ["#/experiments/0", { page: "sessions" }],
    ["#/sessions/4", { page: "session", id: 4 }],
    ["#/sessions/x", { page: "sessions" }],
    ["#/propose?session=2&parameter=ARB_FRONT&clicks=0", { page: "propose" }],
    ["#/propose?session=x&parameter=ARB_FRONT&clicks=1", { page: "propose" }],
    ["#/experiments/abc", { page: "sessions" }],
    ["#/somewhere-else", { page: "sessions" }],
  ];

  it.each(cases)("reads %j", (hash, route) => {
    expect(parseRoute(hash)).toEqual(route);
  });

  it("reads back the link of an experiment", () => {
    expect(parseRoute(experimentHref(7))).toEqual({ page: "experiment", id: 7 });
  });

  it("reads back the link of a session", () => {
    expect(parseRoute(sessionHref(4))).toEqual({ page: "session", id: 4 });
  });

  it("reads back a candidate handed to the propose form, whatever its text", () => {
    const prefill = {
      sessionId: 2,
      parameter: "ARB_FRONT",
      clicks: -1,
      hypothesisKey: "mid_understeer.slow",
      statement: "Front slips 7.7° & rear 3.3? Test: a softer bar = more grip.",
    };

    expect(parseRoute(proposeHref(prefill))).toEqual({ page: "propose", prefill });
  });

  it("carries the gain the profile expects, and drops one that is not a range", () => {
    const plain = {
      sessionId: 2,
      parameter: "ARB_FRONT",
      clicks: -1,
      hypothesisKey: "",
      statement: "Softer.",
    };
    const prefill = { ...plain, gain: { low: 0.012, high: 0.031 } };

    expect(parseRoute(proposeHref(prefill))).toEqual({ page: "propose", prefill });
    const backwards = proposeHref({ ...prefill, gain: { low: 0.05, high: 0.01 } });
    expect(parseRoute(backwards)).toEqual({ page: "propose", prefill: plain });
  });
});
