#!/bin/bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TTL_SECONDS="${1:-${SPEAKER_PROFILE_DB_TTL_SECONDS:-86400}}"

if ! command -v curl >/dev/null 2>&1; then
  echo "Error: curl is required" >&2
  exit 1
fi

if [[ -z "${TTL_SECONDS}" ]]; then
  echo "Usage: TTL seconds required (arg or SPEAKER_PROFILE_DB_TTL_SECONDS)" >&2
  exit 1
fi

echo "Cleaning up speaker profiles older than ${TTL_SECONDS}s via ${BASE_URL}..."
curl -sS -X POST "${BASE_URL}/api/speaker-profiles/cleanup?ttl_seconds=${TTL_SECONDS}"
echo
