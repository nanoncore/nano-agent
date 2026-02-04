#!/usr/bin/env bash
# =============================================================================
# Test: vlan-delete command
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

log_info "Testing vlan-delete for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi

VLAN_ID="${VSOL_VLAN_DELETE_ID:-708}"
VLAN_NAME="${VSOL_VLAN_DELETE_NAME:-vlan708}"
VLAN_DESC="${VSOL_VLAN_DELETE_DESC:-test vlan 708}"

# =============================================================================
# Test 1: Create VLAN (setup)
# =============================================================================
log_info "Test 1: Create VLAN (setup)"

CREATE_OUTPUT=$("$BINARY" vlan-create $CMD_ARGS --vlan-id "$VLAN_ID" --name "$VLAN_NAME" --description "$VLAN_DESC" 2>&1) || {
    log_error "Create failed: $CREATE_OUTPUT"
    exit 1
}

assert_contains "$CREATE_OUTPUT" "created successfully" "Expected create success message"

log_info "Test 1 passed: VLAN created"

# =============================================================================
# Test 2: Delete VLAN
# =============================================================================
log_info "Test 2: Delete VLAN"

DELETE_OUTPUT=$("$BINARY" vlan-delete $CMD_ARGS --vlan-id "$VLAN_ID" --force 2>&1) || {
    log_error "Delete failed: $DELETE_OUTPUT"
    exit 1
}

assert_contains "$DELETE_OUTPUT" "deleted" "Expected delete success message"

log_info "Test 2 passed: VLAN deleted"

# =============================================================================
# Test 3: Verify deletion (get should be null/not found)
# =============================================================================
log_info "Test 3: Verify deletion"

GET_OUTPUT=$("$BINARY" vlan-get $CMD_ARGS --vlan-id "$VLAN_ID" --json 2>&1) || {
    log_error "VLAN get failed: $GET_OUTPUT"
    exit 1
}

if [[ "$GET_OUTPUT" != "null" ]]; then
    log_error "Expected null after deletion, got: $GET_OUTPUT"
    exit 1
fi

log_info "Test 3 passed: VLAN not found after deletion"

# =============================================================================
# Summary
# =============================================================================
log_success "All vlan-delete tests passed for $VENDOR"
exit 0
