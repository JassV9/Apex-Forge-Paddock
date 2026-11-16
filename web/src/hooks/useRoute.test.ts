import { describe, expect, it } from "vitest";

import {
  experimentHref,
  parseRoute,
  proposeHref,
  type Route,
  sessionHref,
  telemetryHref,
} from "./useRoute";

describe("parseRoute", () => {
  const cases: [string, Route][] = [
    ["", { tab: "overview" }],
    ["#/", { tab: "overview" }],
    ["#/sessions/4", { sessionId: 4, tab: "overview" }],
    ["#/sessions/4/diagnosis", { sessionId: 4, tab: "diagnosis" }],
    ["#/sessions/4/garage", { sessionId: 4, tab: "garage" }],
    ["#/sessions/4/experiments", { sessionId: 4, tab: "experiments" }],
    ["#/sessions/4/experiments/12", { sessionId: 4, tab: "experiments", experimentId: 12 }],
    ["#/sessions/4/experiments/0", { sessionId: 4, tab: "experiments" }],
    ["#/sessions/4/telemetry", { sessionId: 4, tab: "telemetry" }],
    [
      "#/sessions/4/telemetry?corner=3&pass=17",
      { sessionId: 4, tab: "telemetry", corner: 3, pass: 17 },
    ],
    ["#/sessions/4/telemetry?corner=x&pass=-1", { sessionId: 4, tab: "telemetry" }],
    ["#/sessions/4/propose", { sessionId: 4, tab: "experiments", propose: "blank" }],
    ["#/sessions/4/nowhere", { sessionId: 4, tab: "overview" }],
    ["#/sessions/x", { tab: "overview" }],
    // Links of the pages before the workspace still lead somewhere.
    ["#/experiments", { tab: "experiments" }],
    ["#/experiments/12", { tab: "experiments", experimentId: 12 }],
    ["#/garage", { tab: "garage" }],
    ["#/propose", { tab: "experiments", propose: "blank" }],
    ["#/propose?session=2&parameter=ARB_FRONT&clicks=0", { tab: "experiments", propose: "blank" }],
    ["#/somewhere-else", { tab: "overview" }],
  ];

  it.each(cases)("reads %j", (hash, route) => {
    expect(parseRoute(hash)).toEqual(route);
  });

  it("reads back the links it makes", () => {
    expect(parseRoute(sessionHref(4))).toEqual({ sessionId: 4, tab: "overview" });
    expect(parseRoute(sessionHref(4, "garage"))).toEqual({ sessionId: 4, tab: "garage" });
    expect(parseRoute(experimentHref(4, 7))).toEqual({
      sessionId: 4,
      tab: "experiments",
      experimentId: 7,
    });
    expect(parseRoute(telemetryHref(4, 9))).toEqual({ sessionId: 4, tab: "telemetry", corner: 9 });
    expect(parseRoute(telemetryHref(4, 9, 31))).toEqual({
      sessionId: 4,
      tab: "telemetry",
      corner: 9,
      pass: 31,
    });
  });

  it("reads back a candidate handed to the propose panel, whatever its text", () => {
    const prefill = {
      sessionId: 2,
      parameter: "ARB_FRONT",
      clicks: -1,
      hypothesisKey: "mid_understeer.slow",
      statement: "Front slips 7.7° & rear 3.3? Test: a softer bar = more grip.",
    };

    expect(parseRoute(proposeHref(2, prefill))).toEqual({
      sessionId: 2,
      tab: "experiments",
      propose: prefill,
    });
  });

  it("takes the session of an old propose link from its query", () => {
    const old = "#/propose?session=2&parameter=ARB_FRONT&clicks=-1&statement=Softer.";

    expect(parseRoute(old)).toEqual({
      sessionId: 2,
      tab: "experiments",
      propose: {
        sessionId: 2,
        parameter: "ARB_FRONT",
        clicks: -1,
        statement: "Softer.",
        hypothesisKey: "",
      },
    });
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
    const panel = (propose: typeof plain) => ({ sessionId: 2, tab: "experiments", propose });

    expect(parseRoute(proposeHref(2, prefill))).toEqual(panel(prefill));
    const backwards = proposeHref(2, { ...prefill, gain: { low: 0.05, high: 0.01 } });
    expect(parseRoute(backwards)).toEqual(panel(plain));
  });
});
