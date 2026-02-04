#!/usr/bin/env bash
# =============================================================================
# Test: port-power command
# =============================================================================
set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/assertions.sh"

# Parse arguments
BINARY=""
VENDOR=""
ADDRESS=""
PORT=""
PROTOCOL=""
USERNAME=""
PASSWORD=""
COMMUNITY=""
CMD_TYPE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --binary) BINARY="$2"; shift 2 ;;
        --vendor) VENDOR="$2"; shift 2 ;;
        --address) ADDRESS="$2"; shift 2 ;;
        --port) PORT="$2"; shift 2 ;;
        --protocol) PROTOCOL="$2"; shift 2 ;;
        --username) USERNAME="$2"; shift 2 ;;
        --password) PASSWORD="$2"; shift 2 ;;
        --community) COMMUNITY="$2"; shift 2 ;;
        --cmd-type) CMD_TYPE="$2"; shift 2 ;;
        *) shift ;;
    esac
done

log_info "Testing port-power for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build CLI command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

PORT_TO_CHECK="${VSOL_PORT_POWER_PON_PORT:-0/1}"

# =============================================================================
# Test 1: JSON output format
# =============================================================================
log_info "Test 1: JSON output format"

OUTPUT=$("$BINARY" port-power $CMD_ARGS --pon-port "$PORT_TO_CHECK" --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

assert_json_valid "$OUTPUT"

PON_PORT=$(echo "$OUTPUT" | jq -r '.pon_port // empty')
assert_equals "$PORT_TO_CHECK" "$PON_PORT" "pon_port should match request"

TX_POWER=$(echo "$OUTPUT" | jq -r '.tx_power_dbm')
assert_not_empty "$TX_POWER" "tx_power_dbm should be present"

log_info "Test 1 passed: JSON output is valid"

# =============================================================================
# Test 2: Table output
# =============================================================================
log_info "Test 2: Table output format"

TABLE_OUTPUT=$("$BINARY" port-power $CMD_ARGS --pon-port "$PORT_TO_CHECK" 2>&1) || {
    log_error "Table output command failed: $TABLE_OUTPUT"
    exit 1
}

assert_not_empty "$TABLE_OUTPUT" "Table output should not be empty"
assert_contains "$TABLE_OUTPUT" "Tx Power" "Table output should include Tx Power"

log_info "Test 2 passed: Table output works"

# =============================================================================
# Summary
# =============================================================================
log_success "All port-power tests passed for $VENDOR"
exit 0
