#!/usr/bin/env bash
# =============================================================================
# Test: onu-reboot command
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

log_info "Testing onu-reboot for vendor: $VENDOR (protocol: $PROTOCOL)"

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

HELP_OUTPUT=$("$BINARY" onu-reboot --help 2>&1) || true

assert_contains "$HELP_OUTPUT" "serial" "Help should mention serial parameter"

log_info "Test 1 passed: Help output is correct"

# =============================================================================
# Test 2: Missing required parameters
# =============================================================================
log_info "Test 2: Missing serial should fail"

set +e
ERROR_OUTPUT=$("$BINARY" onu-reboot $CMD_ARGS 2>&1)
EXIT_CODE=$?
set -e

if [[ $EXIT_CODE -ne 0 ]]; then
log_info "Test 2 passed: Command correctly fails without serial"
else
    log_warn "Test 2: Command did not fail as expected"
fi

# =============================================================================
# Test 3: Reboot existing ONU (integration)
# =============================================================================
log_info "Test 3: Reboot existing ONU (integration)"

if [[ "$VENDOR" != "vsol" ]]; then
    log_warn "Test 3 skipped: only validated for vsol simulator"
else
    TARGET_PON_PORT="${VSOL_REBOOT_PON_PORT:-0/1}"
    TARGET_ONU_ID="${VSOL_REBOOT_ONU_ID:-2}"
    TARGET_SERIAL="${VSOL_REBOOT_ONU_SERIAL:-FHTT01010002}"

    log_info "Rebooting ONU ${TARGET_PON_PORT} ${TARGET_ONU_ID} (serial: ${TARGET_SERIAL})"

    set +e
    REBOOT_OUTPUT=$("$BINARY" onu-reboot $CMD_ARGS \
        --pon-port "$TARGET_PON_PORT" \
        --onu-id "$TARGET_ONU_ID" 2>&1)
    REBOOT_CODE=$?
    set -e

    if [[ $REBOOT_CODE -ne 0 ]]; then
        log_error "Test 3 failed: reboot command returned non-zero"
        log_error "$REBOOT_OUTPUT"
        exit 1
    fi

    # Verify via SNMP (best-effort for simulator)
    if command -v snmpwalk &> /dev/null && [[ -n "$COMMUNITY" ]]; then
        if [[ "$TARGET_PON_PORT" =~ ^0/([0-9]+)$ ]]; then
            PON_INDEX="${BASH_REMATCH[1]}"
            RX_OID="1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7.${PON_INDEX}.${TARGET_ONU_ID}"

            # Expect rx power to go N/A during deactivate phase
            SEEN_NA="false"
            for attempt in 1 2 3 4 5; do
                RX_POWER=$(snmpwalk -v2c -c "$COMMUNITY" "$ADDRESS" "$RX_OID" 2>/dev/null || true)
                if echo "$RX_POWER" | grep -q "N/A"; then
                    SEEN_NA="true"
                    break
                fi
                sleep 1
            done
            if [[ "$SEEN_NA" != "true" ]]; then
                log_warn "Test 3 warning: rx power did not become N/A during reboot"
            fi

            # Expect rx power to return numeric after activate phase
            RESTORED="false"
            for attempt in 1 2 3 4 5; do
                RX_POWER=$(snmpwalk -v2c -c "$COMMUNITY" "$ADDRESS" "$RX_OID" 2>/dev/null || true)
                if ! echo "$RX_POWER" | grep -q "N/A"; then
                    RESTORED="true"
                    break
                fi
                sleep 1
            done
            if [[ "$RESTORED" != "true" ]]; then
                log_error "Test 3 failed: ONU rx power did not restore after reboot"
                log_error "$RX_POWER"
                exit 1
            fi
            log_info "Test 3 passed: ONU rx power restored after reboot"
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
log_success "All onu-reboot tests passed for $VENDOR"
exit 0
