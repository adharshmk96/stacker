#!/usr/bin/env bash
#
# Runs the Go test suite with coverage and prints a readable summary.
#
# Coverage is measured per package: a statement only counts as covered if a test
# in its own package exercised it. That is why packages with no _test.go file
# report 0.0% even when the server tests reach them indirectly. Pass --cross to
# credit coverage across package boundaries instead (go test -coverpkg=./...),
# which raises the numbers but makes them slower to produce.
#
# Usage:
#   scripts/test-coverage.sh [--html] [--cross] [-- <extra go test args>]

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER_DIR="$ROOT/stacker-server"
PROFILE="$SERVER_DIR/coverage.out"

OPEN_HTML=false
CROSS=false
EXTRA_ARGS=()

while [ $# -gt 0 ]; do
  case "$1" in
    --html) OPEN_HTML=true ;;
    --cross) CROSS=true ;;
    --) shift; EXTRA_ARGS=("$@"); break ;;
    *) echo "error: unknown option '$1'" >&2; exit 1 ;;
  esac
  shift
done

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is required — run 'task test' to test in the build image instead" >&2
  exit 1
fi

COVER_ARGS=(-coverprofile="$PROFILE")
if [ "$CROSS" = true ]; then
  COVER_ARGS+=(-coverpkg=./...)
fi

echo "==> running tests with coverage"
cd "$SERVER_DIR"
# Keep the raw run on screen — a failing test has to stay visible — while also
# capturing it, since `go test` already prints the statement-weighted coverage
# per package and recomputing that from the profile would only approximate it.
RESULTS="$(mktemp)"
trap 'rm -f "$RESULTS"' EXIT
go test ./... "${COVER_ARGS[@]}" "${EXTRA_ARGS[@]+"${EXTRA_ARGS[@]}"}" | tee "$RESULTS"

echo
if [ "$CROSS" = true ]; then
  # Under -coverpkg each figure is that package's tests measured against every
  # statement in the program, not against its own — so name it for what it is.
  echo "==> share of all statements reached, by test package"
else
  echo "==> coverage by package"
fi
# Field positions vary — tested packages carry an "ok" and a duration, skipped
# ones do not — so pick the package and percentage out by content, not index.
grep 'coverage:' "$RESULTS" \
  | awk '{
      pkg = ""; pct = ""
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^stacker/) pkg = $i
        if ($i == "coverage:") { pct = $(i + 1); sub(/%$/, "", pct) }
      }
      if (pkg != "" && pct != "") printf "  %6.1f%%  %s\n", pct, pkg
    }' \
  | sort -rn
printf '\n  %s\n' "$(go tool cover -func="$PROFILE" | tail -1 | awk '{ print "total: " $NF }')"

echo
echo "==> functions below 50%"
go tool cover -func="$PROFILE" \
  | grep -v '^total:' \
  | awk '{ pct = $NF; sub(/%$/, "", pct); if (pct + 0 < 50) printf "%8.1f\t%s\t%s\n", pct, $1, $2 }' \
  | sort -n \
  | head -15 \
  | awk -F'\t' '{ printf "  %6.1f%%  %s %s\n", $1, $3, $2 }'

if [ "$OPEN_HTML" = true ]; then
  echo
  echo "==> opening HTML report"
  go tool cover -html="$PROFILE"
else
  echo
  echo "==> profile written to $PROFILE — rerun with --html for the annotated source"
fi
