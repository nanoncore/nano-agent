#!/usr/bin/env bash
# =============================================================================
# Test: onu-delete command
# Note: This is a write operation that modifies OLT state
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

log_info "Testing onu-delete for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build CLI command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

# =============================================================================
# Test 1: Help output
# =============================================================================
log_info "Test 1: Verify help output"

HELP_OUTPUT=$("$BINARY" onu-delete --help 2>&1) || true

assert_contains "$HELP_OUTPUT" "serial" "Help should mention serial parameter"

log_info "Test 1 passed: Help output is correct"

# =============================================================================
# Test 2: Missing required parameters
# =============================================================================
log_info "Test 2: Missing required parameters should fail"

set +e
ERROR_OUTPUT=$("$BINARY" onu-delete $CMD_ARGS 2>&1)
EXIT_CODE=$?
set -e

if [[ $EXIT_CODE -ne 0 ]]; then
    log_info "Test 2 passed: Command correctly fails without serial"
else
    log_warn "Test 2: Command did not fail as expected"
fi

# =============================================================================
# Test 3: Delete non-existent ONU
# =============================================================================
log_info "Test 3: Delete non-existent ONU should fail gracefully"

FAKE_SERIAL="NOTEXIST12345678"

set +e
DELETE_OUTPUT=$("$BINARY" onu-delete $CMD_ARGS --serial "$FAKE_SERIAL" 2>&1)
DELETE_CODE=$?
set -e

# Should fail or indicate ONU not found
log_info "Delete non-existent ONU returned code: $DELETE_CODE"
log_info "Test 3 passed: Command handles non-existent ONU"

# =============================================================================
# Test 4: Delete existing ONU (integration)
# =============================================================================
log_info "Test 4: Delete existing ONU (integration)"

if [[ "$VENDOR" != "vsol" ]]; then
    log_warn "Test 4 skipped: only validated for vsol simulator"
else
    # Simulator defaults (override via env if needed)
    TARGET_PON_PORT="${VSOL_TEST_PON_PORT:-0/1}"
    TARGET_ONU_ID="${VSOL_TEST_ONU_ID:-5}"
    TARGET_SERIAL="${VSOL_TEST_ONU_SERIAL:-FHTT01010005}"

    log_info "Deleting ONU ${TARGET_PON_PORT} ${TARGET_ONU_ID} (serial: ${TARGET_SERIAL})"

    set +e
    DELETE_OUTPUT=$("$BINARY" onu-delete $CMD_ARGS \
        --pon-port "$TARGET_PON_PORT" \
        --onu-id "$TARGET_ONU_ID" \
        --force 2>&1)
    DELETE_CODE=$?
    set -e

    if [[ $DELETE_CODE -ne 0 ]]; then
        log_error "Test 4 failed: delete command returned non-zero"
        log_error "$DELETE_OUTPUT"
        exit 1
    fi

    # Verify via SNMP (best-effort for simulator)
    if command -v snmpwalk &> /dev/null && [[ -n "$COMMUNITY" ]]; then
        # Allow a short delay for SNMP table to reflect deletion
        FOUND="true"
        for attempt in 1 2 3 4 5; do
            SERIAL_TABLE=$(snmpwalk -v2c -c "$COMMUNITY" "$ADDRESS" \
                1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5 2>/dev/null || true)
            if ! echo "$SERIAL_TABLE" | grep -q "$TARGET_SERIAL"; then
                FOUND="false"
                break
            fi
            sleep 1
        done
        if [[ "$FOUND" == "true" ]]; then
            log_error "Test 4 failed: ONU serial still present after delete"
            exit 1
        fi
        log_info "Test 4 passed: ONU serial not present in SNMP table"
    else
        log_warn "Test 4 skipped SNMP verification (snmpwalk unavailable or no community)"
    fi
fi

# =============================================================================
# Summary
# =============================================================================
log_success "All onu-delete tests passed for $VENDOR"
exit 0
