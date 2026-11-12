# ADR 0010: Diagnosis produces hypotheses, and a language model only puts them into words

Status: accepted (2026-11-12)

## Context

v0's advisor read "the front slips more than the rear" and applied the first knob of a rule
table. The audit (`docs/audit-v0.md`) found that the reading measured the driver as much as
the car, and that eight setup versions built on it barely moved it. Phase 4 keeps the rule
table, because its rows are sound rules of thumb, and changes what it is allowed to say. It
also adds a language model, which is the part of this system most able to state something
that was never measured.

## Decision

**The diagnosis is deterministic and lives in the engine** (`engine/paddock_engine/diagnosis`).
`POST /diagnose` takes a session's valid passes and the car's changeable setup items and
returns the same answer for the same input.

**Every number is in the evidence pack.** One entry is the mean of one metric over the valid
passes of one corner class (slow, medium, fast, all), with the number of passes; its id is
`<metric>.<class>`, for example `front_slip_mid_deg.slow`. An entry needs three passes, and
a phase reading counts only when that phase's confidence is 0.5 or more. Hypotheses,
alternatives and explanations refer to entries by id. Nothing downstream states a number
that is not an entry.

**A symptom is a reading beyond a threshold, not a verdict.** Balance is front minus rear
slip per phase against a band of 12 % of the slip level (0.4 degrees at least), as in v0.
Mid-corner balance is read per speed class, because mechanical grip and aero are different
problems; entry and exit over all corners. Lockups, wheelspin and opposite lock on exit use
v0's thresholds. The score is how many thresholds the reading is beyond, and exists only to
order the list.

**Every hypothesis carries two alternatives: driver input and conditions.** Driver input is
marked *supported* for an understeer symptom when the session's own numbers speak for it:

- the front saturation index of that corner class is 0.25 or more (steering still rising
  while lateral g does not follow), or
- front slip is at least 1.5 degrees higher than in fast corners while rear slip differs by
  no more than 0.75 degrees. A balance problem would move the rear too. This is the pattern
  the audit found at COTA.

A supported alternative halves the hypothesis's score: it is still worth testing, after the
ones the driver does not explain. Conditions (tyre temperature, fuel, track grip) are never
supported from one session; the alternative says so and points at the experiment, which is
what separates them. These thresholds are engineering judgement. They have not been checked
against a driver's own reading of a car.

**The rule table gives experiments, not answers.** A row lists knobs by role and axle with a
direction and a one-line reason. For each knob the engine finds the car's items, turns the
direction into clicks using the item's stored scale, and returns a candidate: one item, one
click, the reason, and which way corner time and the symptom's channel should move. Sizes are
not predicted; the sensitivity profile of phase 5 is what will know them. A knob that cannot
be one experiment is listed with the reason: one wheel of a left/right pair, an item at the
end of its range, a knob another symptom rules out, a session without setup data. v0 rows
that need channels Paddock does not compute yet (hot pressures against the tyre's ideal,
bottoming, straight-line speed) are left out.

**A language model explains; it never adds a fact.** `LLMProvider.complete(pack) ->
ExplainReply` has three implementations. `NullProvider` writes template text from the
hypotheses. `GeminiProvider` and `OpenRouterProvider` send the pack and fixed instructions
and must answer with JSON that validates against `contracts/explain_reply.schema.json`. A
reply is shown only if it passes the gate in `explain/validate.py`:

- every hypothesis it ranks and every evidence id it cites is in the pack;
- every snake_case identifier in its prose is in the pack (a metric, an evidence id, a
  hypothesis key or a setup item);
- every number in its prose equals a pack number at the precision it is written with.

A model that computes a difference, converts to a percentage or names a channel the session
never measured fails the gate. So does a transport error or a reply outside the schema. In
each case the template text is shown and `explanation.rejected` says why. The template text
goes through the same gate in the tests. The reply has no numeric field, so nothing a model
writes can enter the ledger as a number.

**Keys and what is sent.** The provider is chosen by `PADDOCK_LLM_PROVIDER` (`none`, `gemini`,
`openrouter`). Unset means Gemini when `GEMINI_API_KEY` is present and no model otherwise, so
a fresh checkout runs with no key. Keys come from the environment or, when the `keyring`
package is installed, the OS keyring (service `paddock`). They are sent in a header, never in
a URL, and never logged, stored or put in a pack. The pack holds metric means, pass counts,
hypothesis text and the labels of setup items: nothing that identifies the driver.

**Core stores every diagnosis and adds nothing to it.** `POST /sessions/{id}/diagnosis` sends
the passes driven on the setup an experiment would start from (all valid passes when the
session has no setup data), leaves out out-laps, and refuses with 409 while the session's
features are still being computed. The reply is stored as it is in the `diagnosis` table;
earlier ones are kept. `POST /experiments` takes an optional `hypothesis_key`: the hypothesis
row then records source `rules`, the phase and corner class, the evidence ids and the
alternatives as they were shown, so the ledger keeps what was known when the experiment was
proposed.

## Consequences

- With no key, no network and no model the system does everything it does with one.
- The gate is strict. A model that writes "two hypotheses" as "2 hypotheses" is rejected
  unless 2 is a pack number. A wrongly rejected reply costs a nicer paragraph; a wrongly
  accepted one would put an invented number in front of the driver.
- The gate checks references and numbers, not reasoning. A reply can cite real numbers and
  still argue badly. That is why the reply is text beside the hypotheses and decides nothing.
- Hosted providers are tested against a recorded transport. No real call to Gemini or
  OpenRouter has been made: **unverified against the live APIs**, including the default
  model ids.

## Rejected

- **Letting the model propose candidates or rank by its own score.** The ranking would no
  longer be reproducible, and "LLM output never enters the ledger as a number" would hold
  only by convention.
- **Asking the model to fix a rejected reply.** A second call doubles cost and latency for a
  paragraph, and the template text is already correct.
- **An HTTP client dependency for two POST requests.** The standard library does it.
- **Hiding the driver alternative unless it is supported.** v0's failure was not asking the
  question. An unsupported alternative costs one line and keeps it asked.
