#!/usr/bin/env bash
# =============================================================================
# Test: vlan-create command
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

log_info "Testing vlan-create for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi

VLAN_ID="${VSOL_VLAN_CREATE_ID:-706}"
VLAN_NAME="${VSOL_VLAN_CREATE_NAME:-vlan706}"
VLAN_DESC="${VSOL_VLAN_CREATE_DESC:-test vlan 706}"

# =============================================================================
# Test 1: Create VLAN
# =============================================================================
log_info "Test 1: Create VLAN"

OUTPUT=$("$BINARY" vlan-create $CMD_ARGS --vlan-id "$VLAN_ID" --name "$VLAN_NAME" --description "$VLAN_DESC" 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

assert_contains "$OUTPUT" "created successfully" "Expected create success message"

log_info "Test 1 passed: VLAN created"

# =============================================================================
# Test 2: Get VLAN (JSON)
# =============================================================================
log_info "Test 2: Get VLAN (JSON)"

GET_OUTPUT=$("$BINARY" vlan-get $CMD_ARGS --vlan-id "$VLAN_ID" --json 2>&1) || {
    log_error "VLAN get failed: $GET_OUTPUT"
    exit 1
}

assert_json_valid "$GET_OUTPUT"
ID_VALUE=$(echo "$GET_OUTPUT" | jq -r '.id // .ID // empty')
assert_equals "$VLAN_ID" "$ID_VALUE" "VLAN ID should match"

log_info "Test 2 passed: VLAN get works"

# =============================================================================
# Test 3: Delete VLAN
# =============================================================================
log_info "Test 3: Delete VLAN"

DELETE_OUTPUT=$("$BINARY" vlan-delete $CMD_ARGS --vlan-id "$VLAN_ID" --force 2>&1) || {
    log_error "VLAN delete failed: $DELETE_OUTPUT"
    exit 1
}

assert_contains "$DELETE_OUTPUT" "deleted" "Expected delete success message"

log_info "Test 3 passed: VLAN deleted"

# =============================================================================
# Summary
# =============================================================================
log_success "All vlan-create tests passed for $VENDOR"
exit 0
