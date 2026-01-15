#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}

pass_count=0
fail_count=0

test_case() {
  local name=$1
  local method=$2
  local path=$3
  local data=${4:-}
  local expected_status=$5
  local expected_error=$6

  local resp
  if [[ -n "${data}" ]]; then
    resp=$(curl -s -w "\n%{http_code}" -X "${method}" -H "Content-Type: application/json" -d "${data}" "${BASE_URL}${path}")
  else
    resp=$(curl -s -w "\n%{http_code}" -X "${method}" "${BASE_URL}${path}")
  fi

  local body=${resp%$'\n'*}
  local status=${resp##*$'\n'}

  if printf "%s" "$body" | python3 -c 'import json,sys
status = int(sys.argv[1])
expected_status = int(sys.argv[2])
expected_error = sys.argv[3]
body = sys.stdin.read().strip()
if status != expected_status:
    print(f"status {status} != {expected_status}")
    raise SystemExit(2)
try:
    payload = json.loads(body) if body else {}
except json.JSONDecodeError as exc:
    print(f"invalid json: {exc}")
    raise SystemExit(3)
if payload.get("success") is not False:
    print("success is not false")
    raise SystemExit(4)
error = payload.get("error")
if not isinstance(error, str):
    print("error is missing or not a string")
    raise SystemExit(5)
if expected_error not in error:
    print(f"error '{error}' does not include '{expected_error}'")
    raise SystemExit(6)
' "$status" "$expected_status" "$expected_error"
  then
    echo "PASS: ${name}"
    pass_count=$((pass_count + 1))
  else
    echo "FAIL: ${name}"
    echo "  status=${status}"
    echo "  body=${body}"
    fail_count=$((fail_count + 1))
  fi
}

# Auth + history endpoints (Keycloak disabled)
test_case "keycloak login method" "GET" "/api/auth/keycloak" "" 405 "Method not allowed"
test_case "keycloak login disabled" "POST" "/api/auth/keycloak" "{}" 503 "Keycloak auth not configured"

# Speaker profiles
test_case "speaker profiles missing session" "GET" "/api/speaker-profiles/" "" 400 "Session ID required"
test_case "speaker cleanup method" "GET" "/api/speaker-profiles/cleanup" "" 405 "Method not allowed"
test_case "speaker cleanup missing ttl" "POST" "/api/speaker-profiles/cleanup" "{}" 400 "ttl_seconds is required"

# Recording endpoints
test_case "recording start method" "GET" "/recording/start" "" 405 "Method not allowed"
test_case "recording start invalid" "POST" "/recording/start" "" 400 "Invalid request"
test_case "recording stop invalid" "POST" "/recording/stop" "" 400 "Invalid request"
test_case "recording stop missing session" "POST" "/recording/stop" '{"sessionId":"missing"}' 404 "Session not found"

# Websocket helpers
test_case "streaming ws hint" "GET" "/ws/stream" "" 200 "Connect to ws://localhost:8003/stream"
test_case "meeting ws missing params" "GET" "/ws/meeting/abc" "" 400 "Missing required parameters"

# Chat endpoints
test_case "chat sessions method" "GET" "/api/chat/sessions" "" 405 "Method not allowed"
test_case "chat sessions invalid" "POST" "/api/chat/sessions" "" 400 "Invalid request"
test_case "chat query invalid" "POST" "/api/chat/query" "" 400 "Invalid request"

# Meetings
test_case "create meeting method" "GET" "/api/meetings" "" 405 "Method not allowed"

# Downloads
# Use a name that's unlikely to exist
random_name="missing-file-$(date +%s).mp4"
test_case "download missing file" "GET" "/download/${random_name}" "" 404 "File not found"

if [[ ${fail_count} -gt 0 ]]; then
  echo "${fail_count} test(s) failed"
  exit 1
fi

echo "All ${pass_count} test(s) passed"
