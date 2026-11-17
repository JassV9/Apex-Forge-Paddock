import { useEffect, useState } from "react";

import {
  type Experiment,
  fetchExperiments,
  fetchTraining,
  type TrainingProgram,
} from "../../api/client";

/** What the Drive panel reads of core, to say what is open on the car; injected in tests. */
export interface OpenWorkApi {
  experiments: () => Promise<Experiment[]>;
  training: () => Promise<TrainingProgram[]>;
}

export const coreOpenWork: OpenWorkApi = { experiments: fetchExperiments, training: fetchTraining };

export interface OpenWork {
  experiments: Experiment[];
  programs: TrainingProgram[];
}

const NOTHING: OpenWork = { experiments: [], programs: [] };

/**
 * The experiments and training programs of core, read once and then every `refreshMs`. When
 * core does not answer there is nothing to say, so the Drive panel works without it.
 */
export function useOpenWork(api: OpenWorkApi, refreshMs: number): OpenWork {
  const [work, setWork] = useState<OpenWork>(NOTHING);
  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const load = () => {
      void Promise.all([api.experiments(), api.training()])
        .then(
          ([experiments, programs]) => {
            if (!cancelled) setWork({ experiments, programs });
          },
          () => undefined,
        )
        .finally(() => {
          if (!cancelled) timer = setTimeout(load, refreshMs);
        });
    };
    load();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [api, refreshMs]);
  return work;
}
