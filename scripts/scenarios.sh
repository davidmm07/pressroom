#!/usr/bin/env bash
# Runs the Postman business scenarios against a running Pressroom and fails if
# any step did not execute. The Postman CLI silently skips request files it
# cannot parse, so a green run alone does not prove every step ran; each step
# logs its number (see the collection's afterResponse script) and this script
# compares those numbers with the request files on disk.
#
#   scripts/scenarios.sh                                 # every scenario
#   scripts/scenarios.sh "02 Refunds wait for a human"   # one scenario
#
# API_URL and WORKER_URL override the environment's apiUrl and workerUrl.
set -euo pipefail
cd "$(dirname "$0")/.."

collection="postman/collections/Pressroom Scenarios"
environment="postman/environments/local.environment.yaml"
scenario="${1:-}"
scope="$collection${scenario:+/$scenario}"
if [ ! -d "$scope" ]; then
  echo "No scenario called \"$scenario\". Available:" >&2
  ls "$collection" >&2
  exit 2
fi

args=(collection run "$collection" -e "$environment")
[ -n "$scenario" ] && args+=(-i "$scenario")
[ -n "${API_URL:-}" ] && args+=(--env-var "apiUrl=$API_URL")
[ -n "${WORKER_URL:-}" ] && args+=(--env-var "workerUrl=$WORKER_URL")

log=$(mktemp)
trap 'rm -f "$log"' EXIT

# The Postman CLI comes from npm; no Postman account is needed for local runs.
set +e
npx --yes postman-cli@1 "${args[@]}" | tee "$log"
status=${PIPESTATUS[0]}
set -e

expected=$(find "$scope" -name '*.request.yaml' -exec basename {} \; | cut -d' ' -f1 | sort -u)
seen=$(grep -oE "'step', '[0-9.]+'" "$log" | cut -d"'" -f4 | sort -u || true)
missing=$(comm -23 <(echo "$expected") <(echo "$seen"))
if [ -n "$missing" ]; then
  echo "These steps never ran; check their .request.yaml files:" >&2
  echo "$missing" >&2
  exit 1
fi
exit "$status"
