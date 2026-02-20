#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER_PKG="./benchmark/cmd/wrk_server"
BIN_PATH="${TMPDIR:-/tmp}/nanite-wrk-server"

THREADS="${WRK_THREADS:-4}"
CONNECTIONS="${WRK_CONNECTIONS:-128}"
DURATION="${WRK_DURATION:-10s}"
TIMEOUT="${WRK_TIMEOUT:-5s}"
PORT_BASE="${PORT_BASE:-18080}"

FRAMEWORKS=("$@")
if [[ ${#FRAMEWORKS[@]} -eq 0 ]]; then
  FRAMEWORKS=("nanite" "httprouter" "chi" "gorilla" "fiber")
fi

if ! command -v wrk >/dev/null 2>&1; then
  echo "wrk not found in PATH. Install wrk and rerun." >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl not found in PATH." >&2
  exit 1
fi

echo "Building benchmark server..."
(
  cd "$ROOT_DIR"
  GOCACHE=/tmp/go-build go build -o "$BIN_PATH" "$SERVER_PKG"
)

TMP_RESULTS="$(mktemp)"
trap 'rm -f "$TMP_RESULTS"' EXIT

wait_for_server() {
  local port="$1"
  local tries=0
  until curl -fsS "http://127.0.0.1:${port}/users" >/dev/null 2>&1; do
    tries=$((tries + 1))
    if [[ $tries -gt 50 ]]; then
      return 1
    fi
    sleep 0.1
  done
}

run_wrk() {
  local port="$1"
  local path="$2"
  wrk --latency -t"$THREADS" -c"$CONNECTIONS" -d"$DURATION" --timeout "$TIMEOUT" "http://127.0.0.1:${port}${path}"
}

extract_metric() {
  local key="$1"
  local output="$2"
  case "$key" in
    req_sec)
      awk '/Requests\/sec:/ {print $2; exit}' <<<"$output"
      ;;
    p50)
      awk '/^[[:space:]]*50%/ {print $2; exit}' <<<"$output"
      ;;
    p99)
      awk '/^[[:space:]]*99%/ {print $2; exit}' <<<"$output"
      ;;
    err_non2xx)
      awk '/Non-2xx or 3xx responses:/ {print $5; exit}' <<<"$output"
      ;;
  esac
}

echo "framework,scenario,requests_sec,p50,p99,non2xx_3xx" >"$TMP_RESULTS"

idx=0
for fw in "${FRAMEWORKS[@]}"; do
  port=$((PORT_BASE + idx))
  idx=$((idx + 1))

  echo
  echo "=== ${fw} on :${port} ==="
  "$BIN_PATH" -framework "$fw" -addr "127.0.0.1:${port}" >/tmp/wrk-server-"$fw".log 2>&1 &
  srv_pid=$!

  cleanup_server() {
    kill "$srv_pid" >/dev/null 2>&1 || true
    wait "$srv_pid" >/dev/null 2>&1 || true
  }
  trap 'cleanup_server; rm -f "$TMP_RESULTS"' EXIT

  if ! wait_for_server "$port"; then
    echo "server ${fw} failed to start (see /tmp/wrk-server-${fw}.log)" >&2
    cleanup_server
    exit 1
  fi

  echo "warmup..."
  run_wrk "$port" "/users" >/dev/null

  for scenario in "static:/users" "param:/user/123"; do
    name="${scenario%%:*}"
    path="${scenario##*:}"
    echo "running ${name} ${path}"
    output="$(run_wrk "$port" "$path")"

    req_sec="$(extract_metric req_sec "$output")"
    p50="$(extract_metric p50 "$output")"
    p99="$(extract_metric p99 "$output")"
    non2xx="$(extract_metric err_non2xx "$output")"
    non2xx="${non2xx:-0}"

    echo "${fw},${name},${req_sec},${p50},${p99},${non2xx}" >>"$TMP_RESULTS"
  done

  cleanup_server
done

echo
echo "Results:"
column -s, -t <"$TMP_RESULTS"
