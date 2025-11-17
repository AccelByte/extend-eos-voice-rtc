#!/usr/bin/env bash
set -euo pipefail

# Runs the load test under resource constraints that emulate a 1 vCPU / 512MiB VM.
# Additional go run flags can be supplied and will be forwarded to the load test.

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

CPUS="${LOADTEST_CPUS:-1}"
MEMORY_SOFT="${LOADTEST_MEMORY:-512m}"
MEMORY_SYSTEMD="${LOADTEST_MEMORY_SYSTEMD:-512M}"
IMAGE="${LOADTEST_IMAGE:-golang:1.24}"

run_with_docker() {
	docker run --rm \
		--cpus="${CPUS}" \
		--memory="${MEMORY_SOFT}" \
		-v "${REPO_ROOT}:/workspace" \
		-w /workspace \
		"${IMAGE}" \
		go run ./cmd/loadtest "$@"
}

run_with_systemd() {
	local args=("$@")
	systemd-run --scope \
		-p MemoryMax="${MEMORY_SYSTEMD}" \
		-p CPUQuota="$((CPUS * 100))%" \
		taskset -c 0 \
		go run ./cmd/loadtest "${args[@]}"
}

if command -v docker >/dev/null 2>&1; then
	run_with_docker "$@"
	exit 0
fi

if command -v systemd-run >/dev/null 2>&1 && command -v taskset >/dev/null 2>&1; then
	run_with_systemd "$@"
	exit 0
fi

echo "Neither Docker nor systemd-run is available. Install Docker or use systemd-run/taskset to enforce resource limits." >&2
exit 1
