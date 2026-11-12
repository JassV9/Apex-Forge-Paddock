#!/bin/sh
# Seeds the read-only demo: diagnoses every recorded session once, from inside the stack.
# Visitors cannot do it themselves, because the public door only lets reads through.
#
#   PADDOCK_CORE_URL   core, as seen from this container (default http://localhost:8080)
#   PADDOCK_SEED_HOLD  1: stay alive when done, for platforms that restart a container
#                      that exits (Azure Container Apps)
set -eu
core="${PADDOCK_CORE_URL:-http://localhost:8080}"

session_ids() {
    curl -fsS "$core/sessions" | grep -o '"id":[0-9]*' | cut -d: -f2
}

echo "waiting for core to ingest the recorded sessions"
until ids="$(session_ids)" && [ -n "$ids" ]; do
    sleep 2
done

for id in $ids; do
    # 409 until the session's features are computed; a stored diagnosis is left alone.
    until curl -fs -o /dev/null "$core/sessions/$id/diagnosis" ||
        curl -fs -o /dev/null -X POST "$core/sessions/$id/diagnosis"; do
        sleep 2
    done
    echo "session $id is diagnosed"
done

if [ "${PADDOCK_SEED_HOLD:-}" = "1" ]; then
    while :; do sleep 3600; done
fi
