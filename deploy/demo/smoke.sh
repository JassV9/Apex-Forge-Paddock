#!/bin/sh
# Checks the read-only demo from outside, as a visitor reaches it: reads are answered,
# every write is refused, and the metrics are not served.
#
#   sh deploy/demo/smoke.sh [base URL]      default http://127.0.0.1:5173
set -eu
base="${1:-http://127.0.0.1:5173}"

status() {
    curl -s -o /dev/null -w '%{http_code}' -X "$1" "$base$2" || true
}

expect() {
    got="$(status "$1" "$2")"
    if [ "$got" != "$3" ]; then
        echo "FAIL $1 $2 answered $got, expected $3"
        exit 1
    fi
    echo "ok   $1 $2 -> $got"
}

first_session() {
    curl -fsS "$base/api/sessions" 2>/dev/null | grep -o '"id":[0-9]*' | head -n 1 | cut -d: -f2
}

echo "waiting for the seeded diagnosis at $base"
tries=0
until id="$(first_session)" && [ -n "$id" ] &&
    [ "$(status GET "/api/sessions/$id/diagnosis")" = "200" ]; do
    tries=$((tries + 1))
    if [ "$tries" -gt 90 ]; then
        echo "FAIL the demo did not come up in 3 minutes"
        exit 1
    fi
    sleep 2
done

expect GET / 200
expect GET /api/sessions 200
expect GET "/api/sessions/$id/diagnosis" 200
expect GET /api/sensitivity 200
expect GET /api/scorecard 200
expect POST "/api/sessions/$id/diagnosis" 403
expect POST /api/experiments 403
expect PUT "/api/sessions/$id" 403
expect PATCH "/api/sessions/$id" 403
expect DELETE "/api/sessions/$id" 403
expect GET /api/metrics 404
echo "the demo answers reads and refuses writes"
