#!/bin/bash
#
# Local E2E test for ResourceAgent against real Redis (Docker).
#
# Prerequisites:
#   - Redis running at localhost:6379 (e.g., docker: ars-redis)
#   - Port 50009 free (mock ServiceDiscovery)
#   - Go toolchain available
#
# Usage:
#   cd ResourceAgent && ./scripts/e2e_test.sh
#
# Environment variables:
#   REDIS_HOST        Redis host (default: 127.0.0.1)
#   REDIS_PORT        Redis port (default: 6379)
#   REDIS_DB          Redis DB   (default: 10)
#   REDIS_CONTAINER   Docker container name for redis-cli (default: ars-redis)
#   SD_PORT           Mock ServiceDiscovery port (default: 50009)
#   RA_RUN_SEC        How long to run ResourceAgent (default: 5)
#

# --- Configuration ---
REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_DB="${REDIS_DB:-10}"
REDIS_CONTAINER="${REDIS_CONTAINER:-ars-redis}"
SD_PORT="${SD_PORT:-50009}"
RA_RUN_SEC="${RA_RUN_SEC:-5}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TMPDIR_E2E=$(mktemp -d)

PASS=0
FAIL=0
TOTAL=0

# E2E test identity (unique to avoid collisions)
E2E_PROCESS="E2E_PROC"
E2E_MODEL="E2E_MODEL"
E2E_EQPID="E2E_$(date +%s)"
E2E_KEY="AgentHealth:resource_agent:${E2E_PROCESS}-${E2E_MODEL}-${E2E_EQPID}"
E2E_METAINFO_KEY="ResourceAgentMetaInfo:${E2E_PROCESS}-${E2E_MODEL}"
EQPINFO_FIELD="${REDIS_HOST}:_"
EQPINFO_VALUE="${E2E_PROCESS}:${E2E_MODEL}:${E2E_EQPID}:LINE1:E2E Test:0"

# PIDs to clean up
MOCK_SD_PID=""
RA_PID=""

# --- Helpers ---
assert() {
    local desc="$1"
    local result="$2"
    TOTAL=$((TOTAL + 1))
    if [ "$result" = "0" ]; then
        echo "  PASS: $desc"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $desc"
        FAIL=$((FAIL + 1))
    fi
}

redis_cli() {
    if command -v redis-cli > /dev/null 2>&1; then
        redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" "$@" 2>/dev/null
    else
        docker exec "$REDIS_CONTAINER" redis-cli -n "$REDIS_DB" "$@" 2>/dev/null
    fi
}

# DB 0 helper — heartbeat and metainfo write to DB 0
redis_cli_db0() {
    if command -v redis-cli > /dev/null 2>&1; then
        redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n 0 "$@" 2>/dev/null
    else
        docker exec "$REDIS_CONTAINER" redis-cli -n 0 "$@" 2>/dev/null
    fi
}

cleanup() {
    echo ""
    echo "--- Cleanup ---"
    # Stop ResourceAgent
    if [ -n "$RA_PID" ] && kill -0 "$RA_PID" 2>/dev/null; then
        kill "$RA_PID" 2>/dev/null
        wait "$RA_PID" 2>/dev/null
        echo "  ResourceAgent stopped"
    fi
    # Stop mock ServiceDiscovery
    if [ -n "$MOCK_SD_PID" ] && kill -0 "$MOCK_SD_PID" 2>/dev/null; then
        kill "$MOCK_SD_PID" 2>/dev/null
        wait "$MOCK_SD_PID" 2>/dev/null
        echo "  Mock ServiceDiscovery stopped"
    fi
    # Remove Redis keys
    redis_cli HDEL EQP_INFO "$EQPINFO_FIELD" > /dev/null 2>&1
    redis_cli DEL "EQP_DIFF:${E2E_EQPID}" > /dev/null 2>&1
    # DB 0 keys (heartbeat + metainfo)
    redis_cli_db0 DEL "$E2E_KEY" > /dev/null 2>&1
    redis_cli_db0 HDEL "$E2E_METAINFO_KEY" "$E2E_EQPID" > /dev/null 2>&1
    echo "  Redis keys cleaned"
    # Remove temp files
    rm -rf "$TMPDIR_E2E"
    echo "  Temp files removed"
}

trap cleanup EXIT

# --- Pre-flight checks ---
echo "=== Pre-flight checks ==="

redis_cli PING | grep -q PONG
assert "Redis reachable at ${REDIS_HOST}:${REDIS_PORT}" $?

# Check port 50009 is free
! lsof -ti:${SD_PORT} > /dev/null 2>&1
assert "Port ${SD_PORT} is free" $?

if [ "$FAIL" -gt 0 ]; then
    echo "Pre-flight failed. Aborting."
    exit 1
fi

# --- Setup ---
echo ""
echo "=== Setup ==="
echo "  EQP_ID: ${E2E_EQPID}"
echo "  Heartbeat key: ${E2E_KEY}"

# 1. Insert EQP_INFO
redis_cli HSET EQP_INFO "$EQPINFO_FIELD" "$EQPINFO_VALUE" > /dev/null
echo "  EQP_INFO inserted"

# 2. Start mock ServiceDiscovery
python3 "$SCRIPT_DIR/mock_servicediscovery.py" --port "$SD_PORT" &
MOCK_SD_PID=$!
sleep 0.5
kill -0 "$MOCK_SD_PID" 2>/dev/null
assert "Mock ServiceDiscovery started (PID: $MOCK_SD_PID)" $?

# 3. Create temp config
mkdir -p "$TMPDIR_E2E/conf/ResourceAgent" "$TMPDIR_E2E/log/ResourceAgent"

cat > "$TMPDIR_E2E/conf/ResourceAgent/ResourceAgent.json" << EOF
{
  "SenderType": "kafkarest",
  "VirtualAddressList": "${REDIS_HOST}",
  "Redis": { "Port": ${REDIS_PORT}, "DB": ${REDIS_DB} },
  "ServiceDiscoveryPort": ${SD_PORT},
  "ResourceMonitorTopic": "e2e_test",
  "TimeDiffSyncInterval": 3600
}
EOF

cat > "$TMPDIR_E2E/conf/ResourceAgent/Monitor.json" << 'EOF'
{
  "Collectors": {
    "CPU": { "Enabled": true, "Interval": "60s" }
  }
}
EOF

cat > "$TMPDIR_E2E/conf/ResourceAgent/Logging.json" << 'EOF'
{
  "Level": "debug",
  "Console": true,
  "File": { "Enabled": false }
}
EOF
echo "  Config files created"

# 4. Build ResourceAgent
echo "  Building ResourceAgent..."
(cd "$PROJECT_DIR" && go build -o "$TMPDIR_E2E/resourceagent" ./cmd/resourceagent 2>&1)
assert "ResourceAgent built" $?

# --- Run ---
echo ""
echo "=== Run ResourceAgent (${RA_RUN_SEC}s) ==="

cd "$TMPDIR_E2E"
./resourceagent \
    -config conf/ResourceAgent/ResourceAgent.json \
    -monitor conf/ResourceAgent/Monitor.json \
    -logging conf/ResourceAgent/Logging.json > "$TMPDIR_E2E/ra.log" 2>&1 &
RA_PID=$!
sleep "$RA_RUN_SEC"

kill -0 "$RA_PID" 2>/dev/null
assert "ResourceAgent process alive after ${RA_RUN_SEC}s" $?

# --- Verify ---
echo ""
echo "=== Verify heartbeat ==="

# Check heartbeat key exists (DB 0)
HB_VAL=$(redis_cli_db0 GET "$E2E_KEY")
[ -n "$HB_VAL" ]
assert "Heartbeat key exists in Redis DB 0" $?

# Check value is numeric (uptime seconds)
if [ -n "$HB_VAL" ]; then
    echo "$HB_VAL" | grep -qE '^OK:[0-9]+$'
    assert "Heartbeat value is OK:N format (value=${HB_VAL})" $?
fi

# Check TTL is set (should be <= 30)
HB_TTL=$(redis_cli_db0 TTL "$E2E_KEY")
[ "$HB_TTL" -gt 0 ] && [ "$HB_TTL" -le 30 ]
assert "Heartbeat TTL valid (${HB_TTL}s, expected 1-30)" $?

# Check heartbeat log entry
grep -q "heartbeat sent" "$TMPDIR_E2E/ra.log" 2>/dev/null
assert "Heartbeat log entry found" $?

# Check EQP_INFO was loaded
grep -q "EQP_INFO loaded from Redis" "$TMPDIR_E2E/ra.log" 2>/dev/null
assert "EQP_INFO loaded from Redis" $?

# --- Verify version metainfo ---
echo ""
echo "=== Verify version metainfo ==="

# Check ResourceAgentMetaInfo key exists in DB 0
MI_VAL=$(redis_cli_db0 HGET "$E2E_METAINFO_KEY" "$E2E_EQPID")
[ -n "$MI_VAL" ]
assert "ResourceAgentMetaInfo key exists in Redis DB 0" $?

# Check value is a version string (not empty)
if [ -n "$MI_VAL" ]; then
    assert "Version value is non-empty (value=${MI_VAL})" 0
fi

# Check metainfo log entry
grep -q "version written to Redis" "$TMPDIR_E2E/ra.log" 2>/dev/null
assert "Version metainfo log entry found" $?

# --- Stop ---
echo ""
echo "=== Stop ResourceAgent ==="
kill "$RA_PID" 2>/dev/null
wait "$RA_PID" 2>/dev/null
RA_EXIT=$?
# 143 = killed by SIGTERM (normal)
[ "$RA_EXIT" -eq 0 ] || [ "$RA_EXIT" -eq 143 ]
assert "ResourceAgent exited cleanly (code: $RA_EXIT)" $?
RA_PID=""

# Stop 후 SHUTDOWN 상태 검증
HB_SHUTDOWN_VAL=$(redis_cli_db0 GET "$E2E_KEY")
if [ -n "$HB_SHUTDOWN_VAL" ]; then
    echo "$HB_SHUTDOWN_VAL" | grep -qE '^SHUTDOWN:[0-9]+$'
    assert "Heartbeat SHUTDOWN value format (value=${HB_SHUTDOWN_VAL})" $?
fi

# Check graceful shutdown log
grep -q "Received shutdown signal" "$TMPDIR_E2E/ra.log" 2>/dev/null
assert "Graceful shutdown logged" $?

# --- Results ---
echo ""
echo "========================================="
echo "Results: $PASS/$TOTAL PASS, $FAIL FAIL"
echo "========================================="

[ "$FAIL" -eq 0 ] && exit 0 || exit 1
