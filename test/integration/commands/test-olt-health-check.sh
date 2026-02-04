#!/usr/bin/env bash
# =============================================================================
# Test: olt-health-check command
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

log_info "Testing olt-health-check for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

# =============================================================================
# Test 1: JSON output format
# =============================================================================
log_info "Test 1: JSON output format"

OUTPUT=$("$BINARY" olt-health-check $CMD_ARGS --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

assert_json_valid "$OUTPUT"
assert_json_has_key "$OUTPUT" "healthy"
assert_json_has_key "$OUTPUT" "message"
assert_json_has_key "$OUTPUT" "port_count"

HEALTHY=$(echo "$OUTPUT" | jq -r '.healthy')
if [[ "$HEALTHY" != "true" && "$HEALTHY" != "false" ]]; then
    log_error "Expected healthy to be boolean, got: $HEALTHY"
    exit 1
fi

PORT_COUNT=$(echo "$OUTPUT" | jq -r '.port_count')
if ! [[ "$PORT_COUNT" =~ ^[0-9]+$ ]]; then
    log_error "Expected port_count to be numeric, got: $PORT_COUNT"
    exit 1
fi

log_info "Test 1 passed: JSON output is valid"

# =============================================================================
# Test 2: Table output format (default)
# =============================================================================
log_info "Test 2: Table output format"

TABLE_OUTPUT=$("$BINARY" olt-health-check $CMD_ARGS 2>&1) || {
    log_error "Table output command failed: $TABLE_OUTPUT"
    exit 1
}

assert_not_empty "$TABLE_OUTPUT" "Table output should not be empty"

log_info "Test 2 passed: Table output works"

# =============================================================================
# Summary
# =============================================================================
log_success "All olt-health-check tests passed for $VENDOR"
exit 0
