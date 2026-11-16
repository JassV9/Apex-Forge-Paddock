import { useEffect, useState } from "react";

import type { PassTrace } from "../../api/client";

export type TraceLoader = (seq: number) => Promise<PassTrace>;

export interface Traces {
  bySeq: Map<number, PassTrace>;
  failed: Set<number>;
  pending: boolean;
}

/** Loads the samples of every pass in `seqs`; what was loaded once is kept by the loader. */
export function useTraces(seqs: number[], load: TraceLoader): Traces {
  const [bySeq, setBySeq] = useState(() => new Map<number, PassTrace>());
  const [failed, setFailed] = useState(() => new Set<number>());
  const wanted = seqs.join(",");

  useEffect(() => {
    let cancelled = false;
    const list = wanted === "" ? [] : wanted.split(",").map(Number);
    for (const seq of list) {
      load(seq).then(
        (trace) => {
          if (!cancelled) setBySeq((known) => new Map(known).set(seq, trace));
        },
        () => {
          if (!cancelled) setFailed((known) => new Set(known).add(seq));
        },
      );
    }
    return () => {
      cancelled = true;
    };
  }, [wanted, load]);

  const pending = seqs.some((seq) => !bySeq.has(seq) && !failed.has(seq));
  return { bySeq, failed, pending };
}

/** A loader that asks core once per pass and remembers the answer. */
export function cachedLoader(fetch: TraceLoader): TraceLoader {
  const known = new Map<number, Promise<PassTrace>>();
  return (seq) => {
    let request = known.get(seq);
    if (request === undefined) {
      request = fetch(seq);
      // A failed request is asked again next time.
      void request.catch(() => {
        known.delete(seq);
      });
      known.set(seq, request);
    }
    return request;
  };
}
