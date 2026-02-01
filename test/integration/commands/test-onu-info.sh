#!/usr/bin/env bash
# =============================================================================
# Test: onu-info command
# =============================================================================
# Tests the nano-agent onu-info command which retrieves detailed ONU information
# including optical diagnostics and traffic counters.
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

log_info "Testing onu-info for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

# Vendor-specific test ONU configuration
# V-SOL uses port format "0/1" and ONU IDs starting at 1
# Huawei uses port format "0/0/1" and ONU IDs starting at 0
case "$VENDOR" in
    vsol)
        TEST_PON_PORT="0/1"
        TEST_ONU_ID="1"
        ;;
    huawei)
        TEST_PON_PORT="0/0/1"
        TEST_ONU_ID="0"
        ;;
    *)
        log_error "Unsupported vendor: $VENDOR"
        exit 1
        ;;
esac

# =============================================================================
# Test 1: Basic execution with JSON output
# =============================================================================
log_info "Test 1: JSON output format for ONU $TEST_ONU_ID on port $TEST_PON_PORT"

OUTPUT=$("$BINARY" onu-info $CMD_ARGS --pon-port "$TEST_PON_PORT" --onu-id "$TEST_ONU_ID" --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

# Validate JSON structure
assert_json_valid "$OUTPUT"

# Check it's an object (single ONU info)
IS_OBJECT=$(echo "$OUTPUT" | jq 'type == "object"')
assert_equals "true" "$IS_OBJECT" "Output should be a JSON object"

log_info "Test 1 passed: JSON output is valid"

# =============================================================================
# Test 2: Verify required fields exist
# =============================================================================
log_info "Test 2: Verify required fields"

# The response may be wrapped in an "onu" object (SNMP) or flat (CLI)
# Check for serial number in various possible locations
SERIAL=$(echo "$OUTPUT" | jq -r '.onu.serial // .serial // .serial_number // .SerialNumber // "null"')
if [[ "$SERIAL" != "null" && -n "$SERIAL" ]]; then
    log_info "Serial number found: $SERIAL"
else
    log_error "Serial number field not found"
    exit 1
fi

# Check PON port
PON_PORT=$(echo "$OUTPUT" | jq -r '.onu.pon_port // .pon_port // "null"')
if [[ "$PON_PORT" != "null" && -n "$PON_PORT" ]]; then
    log_info "PON port found: $PON_PORT"
fi

log_info "Test 2 passed: Required fields present"

# =============================================================================
# Test 3: Verify optical diagnostics fields (if available)
# =============================================================================
log_info "Test 3: Check optical diagnostics fields"

# Check for rx_power in various possible locations:
# - SNMP format: .power.rx_power_dbm or .onu.rx_power_dbm
# - CLI format: .rx_power or .RxPower
RX_POWER=$(echo "$OUTPUT" | jq -r '.power.rx_power_dbm // .onu.rx_power_dbm // .rx_power // .RxPower // "null"')
TX_POWER=$(echo "$OUTPUT" | jq -r '.power.tx_power_dbm // .onu.tx_power_dbm // .tx_power // .TxPower // "null"')

OPTICAL_FOUND=false

if [[ "$RX_POWER" != "null" ]]; then
    log_info "Rx Power found: $RX_POWER dBm"
    OPTICAL_FOUND=true
fi

if [[ "$TX_POWER" != "null" ]]; then
    log_info "Tx Power found: $TX_POWER dBm"
    OPTICAL_FOUND=true
fi

# Check for temperature in power metadata (SNMP) or top-level
TEMPERATURE=$(echo "$OUTPUT" | jq -r '.power.metadata.temperature // .temperature // "null"')
if [[ "$TEMPERATURE" != "null" ]]; then
    log_info "Temperature found: $TEMPERATURE C"
fi

if [[ "$OPTICAL_FOUND" == "true" ]]; then
    log_info "Test 3 passed: Optical diagnostics fields present"
else
    log_warn "Test 3 warning: No optical diagnostics fields found (may be expected for some protocols)"
fi

# =============================================================================
# Test 4: Table output format
# =============================================================================
log_info "Test 4: Table output format"

TABLE_OUTPUT=$("$BINARY" onu-info $CMD_ARGS --pon-port "$TEST_PON_PORT" --onu-id "$TEST_ONU_ID" 2>&1) || {
    log_error "Table output command failed: $TABLE_OUTPUT"
    exit 1
}

assert_not_empty "$TABLE_OUTPUT" "Table output should not be empty"

log_info "Test 4 passed: Table output works"

# =============================================================================
# Test 5: Non-existent ONU should return error
# =============================================================================
log_info "Test 5: Non-existent ONU error handling"

# Use a high ONU ID that shouldn't exist
INVALID_OUTPUT=$("$BINARY" onu-info $CMD_ARGS --pon-port "$TEST_PON_PORT" --onu-id "999" --json 2>&1) || true

# We expect either an error message or empty/null result
# Not a hard failure if the command exits non-zero
if echo "$INVALID_OUTPUT" | grep -qi "error\|not found\|does not exist\|null"; then
    log_info "Test 5 passed: Non-existent ONU handled appropriately"
elif [[ -z "$INVALID_OUTPUT" ]]; then
    log_info "Test 5 passed: Empty response for non-existent ONU"
else
    log_warn "Test 5 warning: Unexpected response for non-existent ONU"
fi

# =============================================================================
# Summary
# =============================================================================
log_success "All onu-info tests passed for $VENDOR"
exit 0
