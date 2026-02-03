#!/usr/bin/env bash
# =============================================================================
# Test: diagnose command (ONU diagnostics)
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

log_info "Testing diagnose for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

if [[ "$VENDOR" != "vsol" ]]; then
    log_warn "Diagnostics test skipped: only validated for vsol"
    exit 0
fi

TEST_PON_PORT="${VSOL_DIAG_PON_PORT:-0/1}"
TEST_ONU_ID="${VSOL_DIAG_ONU_ID:-2}"

# =============================================================================
# Test 1: JSON output format for diagnostics
# =============================================================================
log_info "Test 1: JSON output format for ONU $TEST_ONU_ID on port $TEST_PON_PORT"

OUTPUT=$("$BINARY" diagnose $CMD_ARGS --pon-port "$TEST_PON_PORT" --onu-id "$TEST_ONU_ID" --json 2> /tmp/diag.stderr) || {
    log_error "Command failed with output: $OUTPUT"
    if [[ -s /tmp/diag.stderr ]]; then
        log_error "Stderr:"
        cat /tmp/diag.stderr
    fi
    exit 1
}

assert_json_valid "$OUTPUT"

IS_OBJECT=$(echo "$OUTPUT" | jq 'type == "object"')
assert_equals "true" "$IS_OBJECT" "Output should be a JSON object"

log_info "Test 1 passed: JSON output is valid"

# =============================================================================
# Test 2: Required diagnostics fields
# =============================================================================
log_info "Test 2: Verify required fields"

HAS_SERIAL=$(echo "$OUTPUT" | jq 'has("serial") or has("Serial") or (.onu | has("serial"))')
if [[ "$HAS_SERIAL" != "true" ]]; then
    log_error "Serial field not present in output"
    exit 1
fi

PON_PORT=$(echo "$OUTPUT" | jq -r '.pon_port // .PONPort // "null"')
if [[ "$PON_PORT" == "null" || -z "$PON_PORT" ]]; then
    log_error "PON port field not found"
    exit 1
fi

ONU_ID=$(echo "$OUTPUT" | jq -r '.onu_id // .ONUID // "null"')
if [[ "$ONU_ID" == "null" || -z "$ONU_ID" ]]; then
    log_error "ONU ID field not found"
    exit 1
fi

log_info "Test 2 passed: Required fields present"

# =============================================================================
# Test 3: Optical diagnostics fields
# =============================================================================
log_info "Test 3: Verify optical diagnostics"

RX_POWER=$(echo "$OUTPUT" | jq -r '.power.rx_power_dbm // .power.RxPowerDBm // "null"')
TX_POWER=$(echo "$OUTPUT" | jq -r '.power.tx_power_dbm // .power.TxPowerDBm // "null"')
if [[ "$RX_POWER" == "null" || "$TX_POWER" == "null" ]]; then
    log_error "Optical power fields missing"
    exit 1
fi

log_info "Test 3 passed: Optical diagnostics fields present"

# =============================================================================
# Summary
# =============================================================================
log_success "All diagnose tests passed for $VENDOR"
exit 0
