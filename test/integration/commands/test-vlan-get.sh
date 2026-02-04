#!/usr/bin/env bash
# =============================================================================
# Test: vlan-get command
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

log_info "Testing vlan-get for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi

VLAN_ID="${VSOL_VLAN_ID:-702}"

# =============================================================================
# Test 1: JSON output format
# =============================================================================
log_info "Test 1: JSON output format"

OUTPUT=$("$BINARY" vlan-get $CMD_ARGS --vlan-id "$VLAN_ID" --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

assert_json_valid "$OUTPUT"
JSON_TYPE=$(echo "$OUTPUT" | jq -r 'type')
assert_equals "object" "$JSON_TYPE" "Expected JSON object output"

ID_VALUE=$(echo "$OUTPUT" | jq -r '.id // .ID // empty')
assert_equals "$VLAN_ID" "$ID_VALUE" "VLAN ID should match"

log_info "Test 1 passed: JSON output is valid"

# =============================================================================
# Test 2: Table output format (default)
# =============================================================================
log_info "Test 2: Table output format"

TABLE_OUTPUT=$("$BINARY" vlan-get $CMD_ARGS --vlan-id "$VLAN_ID" 2>&1) || {
    log_error "Table output command failed: $TABLE_OUTPUT"
    exit 1
}

assert_not_empty "$TABLE_OUTPUT" "Table output should not be empty"
assert_contains "$TABLE_OUTPUT" "VLAN Details" "Expected table header"
assert_contains "$TABLE_OUTPUT" "$VLAN_ID" "Expected VLAN ID in table output"

log_info "Test 2 passed: Table output works"

# =============================================================================
# Summary
# =============================================================================
log_success "All vlan-get tests passed for $VENDOR"
exit 0
