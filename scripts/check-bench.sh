#!/usr/bin/env bash
# check-bench.sh — Validate benchmark results against performance gates.
#
# Usage: check-bench.sh <bench-output-file>
#
# Gates:
#   BenchmarkParseFile10k    < 50 ms   (50,000,000 ns)
#   BenchmarkScanCollection500 < 500 ms (500,000,000 ns)
#
# The script parses Go benchmark output lines of the form:
#   BenchmarkFoo-N   123   456789 ns/op
# and extracts the ns/op value for each named benchmark.

set -euo pipefail

BENCH_FILE="${1:-bench.txt}"
FAIL=0

# Extract the ns/op for a named benchmark. Returns 0 if not found.
get_nsop() {
    local name="$1"
    grep -E "^${name}-" "${BENCH_FILE}" 2>/dev/null \
        | awk '{print $3}' \
        | head -1
}

check_gate() {
    local name="$1"
    local limit_ns="$2"
    local limit_label="$3"

    local nsop
    nsop=$(get_nsop "${name}")

    if [[ -z "${nsop}" ]]; then
        echo "SKIP  ${name}: not found in ${BENCH_FILE}"
        return
    fi

    # Use awk for integer comparison (nsop may be a large integer)
    if awk -v v="${nsop}" -v lim="${limit_ns}" 'BEGIN{exit !(v+0 > lim+0)}'; then
        echo "FAIL  ${name}: ${nsop} ns/op exceeds limit ${limit_ns} ns/op (${limit_label})"
        FAIL=1
    else
        local ms
        ms=$(awk -v v="${nsop}" 'BEGIN{printf "%.1f", v/1000000}')
        echo "PASS  ${name}: ${ms} ms < ${limit_label}"
    fi
}

echo "=== Benchmark Gates ==="
check_gate "BenchmarkParseFile10k"      50000000  "50 ms"
check_gate "BenchmarkScanCollection500" 500000000 "500 ms"
echo "======================"

if [[ "${FAIL}" -ne 0 ]]; then
    echo "One or more benchmark gates FAILED."
    exit 1
fi
echo "All benchmark gates passed."
