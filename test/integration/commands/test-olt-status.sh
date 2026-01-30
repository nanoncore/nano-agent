#!/usr/bin/env bash
# =============================================================================
# Test: olt-status command
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

log_info "Testing olt-status for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

# =============================================================================
# Test 1: Basic execution with JSON output
# =============================================================================
log_info "Test 1: JSON output format"

OUTPUT=$("$BINARY" olt-status $CMD_ARGS --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

# Validate JSON structure
assert_json_valid "$OUTPUT"
assert_json_has_key "$OUTPUT" "vendor"
assert_json_has_key "$OUTPUT" "is_reachable"

# Check vendor matches
RESPONSE_VENDOR=$(echo "$OUTPUT" | jq -r '.vendor // empty' | tr '[:upper:]' '[:lower:]')
if [[ -n "$RESPONSE_VENDOR" ]]; then
    assert_equals "$VENDOR" "$RESPONSE_VENDOR" "Vendor should match"
fi

# Check reachability
IS_REACHABLE=$(echo "$OUTPUT" | jq -r '.is_reachable // false')
assert_equals "true" "$IS_REACHABLE" "OLT should be reachable"

log_info "Test 1 passed: JSON output is valid"

# =============================================================================
# Test 2: Table output format (default)
# =============================================================================
log_info "Test 2: Table output format"

TABLE_OUTPUT=$("$BINARY" olt-status $CMD_ARGS 2>&1) || {
    log_error "Table output command failed: $TABLE_OUTPUT"
    exit 1
}

# Check for expected content in table output
assert_not_empty "$TABLE_OUTPUT" "Table output should not be empty"

log_info "Test 2 passed: Table output works"

# =============================================================================
# Summary
# =============================================================================
log_success "All olt-status tests passed for $VENDOR"
exit 0
