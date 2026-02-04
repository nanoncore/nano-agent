#!/usr/bin/env bash
# =============================================================================
# Test: service-port-delete command
# Note: This is a write operation
# =============================================================================
set -euo pipefail

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

log_info "Testing service-port-delete for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build CLI command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

PON_PORT="${VSOL_SERVICE_PORT_DELETE_PON_PORT:-0/1}"
ONU_ID="${VSOL_SERVICE_PORT_DELETE_ONU_ID:-2}"
VLAN_ID="${VSOL_SERVICE_PORT_DELETE_VLAN:-704}"
GEMPORT_ID="${VSOL_SERVICE_PORT_DELETE_GEMPORT:-1}"
USER_VLAN="${VSOL_SERVICE_PORT_DELETE_USER_VLAN:-$VLAN_ID}"

# =============================================================================
# Test 1: Add service port (precondition)
# =============================================================================
log_info "Test 1: Add service port on $PON_PORT ONU $ONU_ID VLAN $VLAN_ID"

ADD_OUTPUT=$("$BINARY" service-port-add $CMD_ARGS \
    --pon-port "$PON_PORT" --ont-id "$ONU_ID" --vlan-id "$VLAN_ID" \
    --gemport "$GEMPORT_ID" --user-vlan "$USER_VLAN" 2>&1) || {
    log_error "Service port add failed: $ADD_OUTPUT"
    exit 1
}

assert_contains "$ADD_OUTPUT" "Service port added successfully" "Expected success message"

log_info "Test 1 passed: Service port added"

# =============================================================================
# Test 2: Verify via service-port-list (SNMP)
# =============================================================================
log_info "Test 2: Verify service port in SNMP list"

SNMP_PORT="${VSOL_SNMP_PORT:-161}"
SNMP_COMMUNITY="${COMMUNITY:-${SNMP_COMMUNITY:-public}}"
SNMP_ARGS="--vendor $VENDOR --address $ADDRESS --port $SNMP_PORT --protocol snmp --community $SNMP_COMMUNITY"

LIST_OUTPUT=$("$BINARY" service-port-list $SNMP_ARGS --json 2>&1) || {
    log_error "Service port list failed: $LIST_OUTPUT"
    exit 1
}

assert_json_valid "$LIST_OUTPUT"

MATCH_COUNT=$(echo "$LIST_OUTPUT" | jq "[.[] | select(.interface==\"$PON_PORT\" and .ont_id==$ONU_ID and .vlan==$VLAN_ID)] | length")
if [[ "$MATCH_COUNT" -le 0 ]]; then
    log_error "Expected service port entry in SNMP list"
    log_error "Value: $MATCH_COUNT"
    exit 1
fi

log_info "Test 2 passed: Service port found in SNMP list"

# =============================================================================
# Test 3: Delete service port
# =============================================================================
log_info "Test 3: Delete service port on $PON_PORT ONU $ONU_ID"

DELETE_OUTPUT=$("$BINARY" service-port-delete $CMD_ARGS \
    --pon-port "$PON_PORT" --ont-id "$ONU_ID" 2>&1) || {
    log_error "Service port delete failed: $DELETE_OUTPUT"
    exit 1
}

assert_contains "$DELETE_OUTPUT" "Service port deleted successfully" "Expected success message"

log_info "Test 3 passed: Service port deleted"

# =============================================================================
# Test 4: Verify deletion via service-port-list (SNMP)
# =============================================================================
log_info "Test 4: Verify service port removed from SNMP list"

LIST_OUTPUT_AFTER=$("$BINARY" service-port-list $SNMP_ARGS --json 2>&1) || {
    log_error "Service port list failed: $LIST_OUTPUT_AFTER"
    exit 1
}

assert_json_valid "$LIST_OUTPUT_AFTER"

MATCH_COUNT_AFTER=$(echo "$LIST_OUTPUT_AFTER" | jq "[.[] | select(.interface==\"$PON_PORT\" and .ont_id==$ONU_ID and .vlan==$VLAN_ID)] | length")
if [[ "$MATCH_COUNT_AFTER" -gt 0 ]]; then
    log_error "Expected service port entry to be removed from SNMP list"
    log_error "Value: $MATCH_COUNT_AFTER"
    exit 1
fi

log_info "Test 4 passed: Service port removed from SNMP list"

# =============================================================================
# Summary
# =============================================================================
log_success "All service-port-delete tests passed for $VENDOR"
exit 0
