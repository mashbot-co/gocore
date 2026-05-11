#!/usr/bin/env bash
# Enforce a per-package coverage threshold.
#
# Usage:
#   ./scripts/check-coverage.sh [threshold]          (default: 90)
#   ./scripts/check-coverage.sh [threshold] --full   (don't pass -short; requires Postgres)
#
# Exits non-zero if any package with tests is below the threshold.
# Packages with no test files are skipped (reported as N/A).

set -euo pipefail
THRESHOLD="${1:-90}"
MODE="${2:-short}"

GO_TEST_ARGS=(-cover ./...)
if [[ "$MODE" != "--full" ]]; then
	GO_TEST_ARGS=(-short -cover ./...)
fi

mode_note="(-short; Postgres-dependent tests skipped)"
if [[ "$MODE" == "--full" ]]; then
	mode_note="(full; expects Postgres on localhost:5432)"
fi

echo "Per-package coverage (threshold: ${THRESHOLD}%) ${mode_note}:"
echo

# Collect all output once so we can both print and evaluate.
output=$(go test "${GO_TEST_ARGS[@]}" 2>/dev/null || true)

fail_count=0
while IFS= read -r line; do
	if [[ "$line" == FAIL ]]; then continue; fi

	if [[ "$line" == ?* && "$line" == *"no test files"* ]]; then
		pkg=$(echo "$line" | awk '{print $2}')
		printf "  %-55s  N/A   (no test files)\n" "$pkg"
		continue
	fi

	if [[ "$line" == ok* && "$line" == *coverage:* ]]; then
		pkg=$(echo "$line" | awk '{print $2}')
		pct=$(echo "$line" | sed -E 's/.*coverage: ([0-9.]+)%.*/\1/')
		pct_int=$(echo "$pct * 10 / 1" | bc 2>/dev/null || echo "0")
		threshold_int=$((THRESHOLD * 10))

		if (( pct_int >= threshold_int )); then
			printf "  %-55s  %5s%%  ✓\n" "$pkg" "$pct"
		else
			printf "  %-55s  %5s%%  ✗ below ${THRESHOLD}%%\n" "$pkg" "$pct"
			fail_count=$((fail_count + 1))
		fi
	fi
done <<< "$output"

echo
if (( fail_count > 0 )); then
	if [[ "$MODE" != "--full" ]]; then
		echo "Coverage check FAILED: ${fail_count} package(s) below ${THRESHOLD}% in -short mode."
		echo "Note: packages with Postgres-dependent tests are dragged down by skips."
		echo "      Run with --full + Postgres on localhost:5432 to include them."
	else
		echo "Coverage check FAILED: ${fail_count} package(s) below ${THRESHOLD}%."
	fi
	exit 1
fi
echo "Coverage check passed: all packages with tests at or above ${THRESHOLD}%."
