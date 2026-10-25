#!/usr/bin/env bash
# Run the litmus WebDAV compliance suite against a local `camel serve` instance.
#
# Usage:
#   scripts/litmus.sh                 # basic, copymove, props, http
#   TESTS="basic" scripts/litmus.sh   # one suite
#   ADDR=127.0.0.1:9100 scripts/litmus.sh
#
# Environment:
#   TESTS     space separated litmus suites (default: "basic copymove props http")
#   ADDR      listen address (default: 127.0.0.1:9000)
#   BIN       camel binary (default: <repo>/bin/camel)
#   OUTDIR    log directory (default: <repo>/.litmus)
#   SKIP_BUILD=1 to reuse an existing binary
#
# The server request log and the per-suite litmus logs are kept in OUTDIR.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADDR="${ADDR:-127.0.0.1:9000}"
TESTS="${TESTS:-basic copymove props http}"
BIN="${BIN:-$ROOT/bin/camel}"
OUTDIR="${OUTDIR:-$ROOT/.litmus}"
URL="http://$ADDR/"

mkdir -p "$OUTDIR"

if [[ "${SKIP_BUILD:-0}" != "1" ]]; then
  # Fall back to a writable build cache when the default one is read-only.
  if [[ -z "${GOCACHE:-}" ]]; then
    default_cache="$(go env GOCACHE 2>/dev/null || true)"
    if [[ -z "$default_cache" ]] || ! mkdir -p "$default_cache" 2>/dev/null || [[ ! -w "$default_cache" ]]; then
      export GOCACHE="${TMPDIR:-/tmp}/camel-gocache"
    fi
  fi
  echo "==> building $BIN"
  (cd "$ROOT" && go build -o "$BIN" ./cmd/camel) || exit 1
fi

if ! command -v litmus >/dev/null 2>&1; then
  echo "litmus not installed: apt install litmus" >&2
  exit 1
fi

PORT="${ADDR##*:}"
if curl -s -o /dev/null -m 1 "http://127.0.0.1:$PORT/"; then
  echo "port $PORT already in use" >&2
  exit 1
fi

echo "==> starting camel serve on $ADDR"
CAMEL_WEBDAV_DEBUG=1 "$BIN" serve lt --addr "$ADDR" >"$OUTDIR/server.log" 2>&1 &
SERVER_PID=$!
cleanup() {
  kill "$SERVER_PID" 2>/dev/null
  wait "$SERVER_PID" 2>/dev/null
}
trap cleanup EXIT INT TERM

for _ in $(seq 1 40); do
  if curl -s -o /dev/null -m 1 -X OPTIONS "$URL"; then break; fi
  sleep 0.5
done
if ! curl -s -o /dev/null -m 2 -X OPTIONS "$URL"; then
  echo "server did not start; see $OUTDIR/server.log" >&2
  exit 1
fi

failed=0
for suite in $TESTS; do
  log="$OUTDIR/$suite.log"
  echo "==> litmus suite: $suite"
  TESTS="$suite" litmus -k -n "$URL" >"$log" 2>&1
  rc=$?
  summary="$(grep -aE '^<- summary' "$log" | tail -1)"
  echo "    ${summary:-litmus exit $rc (see $log)}"
  grep -aE 'FAIL|WARNING|fatal|summary' "$log" | grep -avE '^<- summary' | sed 's/^/    /' || true
  if [[ $rc -ne 0 ]] || ! grep -aq '0 failed' "$log"; then
    failed=1
  fi
done

echo "==> logs in $OUTDIR (request log: server.log)"
exit "$failed"
