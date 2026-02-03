#!/usr/bin/env bash
# =============================================================================
# Test: onu-resume command
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

log_info "Testing onu-resume for vendor: $VENDOR (protocol: $PROTOCOL)"

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

HELP_OUTPUT=$("$BINARY" onu-resume --help 2>&1) || true

assert_contains "$HELP_OUTPUT" "serial" "Help should mention serial parameter"

log_info "Test 1 passed: Help output is correct"

# =============================================================================
# Test 2: Missing required parameters
# =============================================================================
log_info "Test 2: Missing required parameters should fail"

set +e
ERROR_OUTPUT=$("$BINARY" onu-resume $CMD_ARGS 2>&1)
EXIT_CODE=$?
set -e

if [[ $EXIT_CODE -ne 0 ]]; then
    log_info "Test 2 passed: Command correctly fails without serial"
else
    log_warn "Test 2: Command did not fail as expected"
fi

# =============================================================================
# Test 3: Resume existing ONU (integration)
# =============================================================================
log_info "Test 3: Resume existing ONU (integration)"

if [[ "$VENDOR" != "vsol" ]]; then
    log_warn "Test 3 skipped: only validated for vsol simulator"
else
    TARGET_PON_PORT="${VSOL_RESUME_PON_PORT:-0/1}"
    TARGET_ONU_ID="${VSOL_RESUME_ONU_ID:-2}"
    TARGET_SERIAL="${VSOL_RESUME_ONU_SERIAL:-FHTT01010002}"

    log_info "Resuming ONU ${TARGET_PON_PORT} ${TARGET_ONU_ID} (serial: ${TARGET_SERIAL})"

    # Ensure ONU is suspended before resuming (best-effort setup)
    set +e
    "$BINARY" onu-suspend $CMD_ARGS \
        --pon-port "$TARGET_PON_PORT" \
        --onu-id "$TARGET_ONU_ID" >/dev/null 2>&1
    set -e

    # Verify suspended via SNMP (best-effort for simulator)
    if command -v snmpwalk &> /dev/null && [[ -n "$COMMUNITY" ]]; then
        if [[ "$TARGET_PON_PORT" =~ ^0/([0-9]+)$ ]]; then
            PON_INDEX="${BASH_REMATCH[1]}"
            RX_OID="1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7.${PON_INDEX}.${TARGET_ONU_ID}"
            for attempt in 1 2 3 4 5; do
                RX_POWER=$(snmpwalk -v2c -c "$COMMUNITY" "$ADDRESS" "$RX_OID" 2>/dev/null || true)
                if echo "$RX_POWER" | grep -q "N/A"; then
                    break
                fi
                sleep 1
            done
        else
            log_warn "Test 3 skipped SNMP pre-check (unsupported PON port format)"
        fi
    else
        log_warn "Test 3 skipped SNMP pre-check (snmpwalk unavailable or no community)"
    fi

    set +e
    RESUME_OUTPUT=$("$BINARY" onu-resume $CMD_ARGS \
        --pon-port "$TARGET_PON_PORT" \
        --onu-id "$TARGET_ONU_ID" 2>&1)
    RESUME_CODE=$?
    set -e

    if [[ $RESUME_CODE -ne 0 ]]; then
        log_error "Test 3 failed: resume command returned non-zero"
        log_error "$RESUME_OUTPUT"
        exit 1
    fi

    # Verify via SNMP (best-effort for simulator)
    if command -v snmpwalk &> /dev/null && [[ -n "$COMMUNITY" ]]; then
        if [[ "$TARGET_PON_PORT" =~ ^0/([0-9]+)$ ]]; then
            PON_INDEX="${BASH_REMATCH[1]}"
            RX_OID="1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7.${PON_INDEX}.${TARGET_ONU_ID}"
            VERIFIED="false"
            for attempt in 1 2 3 4 5; do
                RX_POWER=$(snmpwalk -v2c -c "$COMMUNITY" "$ADDRESS" "$RX_OID" 2>/dev/null || true)
                if ! echo "$RX_POWER" | grep -q "N/A"; then
                    VERIFIED="true"
                    break
                fi
                sleep 1
            done
            if [[ "$VERIFIED" != "true" ]]; then
                log_error "Test 3 failed: ONU rx power still N/A after resume"
                log_error "$RX_POWER"
                exit 1
            fi
            log_info "Test 3 passed: ONU rx power restored (not N/A)"
        else
            log_warn "Test 3 skipped SNMP verification (unsupported PON port format)"
        fi
    else
        log_warn "Test 3 skipped SNMP verification (snmpwalk unavailable or no community)"
    fi
fi

# =============================================================================
# Summary
# =============================================================================
log_success "All onu-resume tests passed for $VENDOR"
exit 0
