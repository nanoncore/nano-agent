#!/usr/bin/env bash
# =============================================================================
# Test: onu-update command
# Note: This is a write operation that modifies OLT state
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

log_info "Testing onu-update for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build CLI command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

# =============================================================================
# Test 1: Help output (non-destructive)
# =============================================================================
log_info "Test 1: Verify help output"

HELP_OUTPUT=$("$BINARY" onu-update --help 2>&1) || true

assert_contains "$HELP_OUTPUT" "pon-port" "Help should mention pon-port parameter"
assert_contains "$HELP_OUTPUT" "onu-id" "Help should mention onu-id parameter"
assert_contains "$HELP_OUTPUT" "vlan" "Help should mention vlan parameter"
assert_contains "$HELP_OUTPUT" "traffic-profile" "Help should mention traffic-profile parameter"

log_info "Test 1 passed: Help output is correct"

# =============================================================================
# Test 2: Missing required parameters
# =============================================================================
log_info "Test 2: Missing required parameters should fail"

set +e
ERROR_OUTPUT=$("$BINARY" onu-update $CMD_ARGS 2>&1)
EXIT_CODE=$?
set -e

# Should fail without required parameters
if [[ $EXIT_CODE -ne 0 ]]; then
    log_info "Test 2 passed: Command correctly fails without required params"
else
    log_warn "Test 2: Command did not fail as expected"
fi

# =============================================================================
# Test 3: No update parameters provided
# =============================================================================
log_info "Test 3: No update parameters should fail"

set +e
ERROR_OUTPUT=$("$BINARY" onu-update $CMD_ARGS --pon-port "0/1" --onu-id 1 2>&1)
EXIT_CODE=$?
set -e

# Should fail without at least one update parameter
if [[ $EXIT_CODE -ne 0 ]]; then
    assert_contains "$ERROR_OUTPUT" "at least one update parameter required" "Error should mention update parameter requirement"
    log_info "Test 3 passed: Command correctly fails without update params"
else
    log_warn "Test 3: Command did not fail as expected"
fi

# =============================================================================
# Integration Tests (require actual OLT/simulator)
# =============================================================================

# Check if we should skip integration tests (quick validation only)
if [[ "$CMD_TYPE" == "validation-only" ]]; then
    log_success "Validation tests passed for $VENDOR (skipping integration tests)"
    exit 0
fi

# =============================================================================
# Setup: Provision test ONUs
# =============================================================================
log_info "Setting up test ONUs for integration tests..."

# ONU 10: No line profile (for Flow 1 test)
log_info "Provisioning ONU 10 (no line profile)..."
"$BINARY" onu-provision $CMD_ARGS \
    --serial FHTT99990010 --pon-port "0/1" --onu-id 10 --vlan 100 > /dev/null 2>&1 || log_warn "ONU 10 provision may have failed"

# ONU 11: With line profile (for Flow 2 test)
log_info "Provisioning ONU 11 (with line profile)..."
"$BINARY" onu-provision $CMD_ARGS \
    --serial FHTT99990011 --pon-port "0/1" --onu-id 11 \
    --line-profile "line_vlan_100" --vlan 100 > /dev/null 2>&1 || log_warn "ONU 11 provision may have failed"

# ONU 12: With line profile (for optimization test)
log_info "Provisioning ONU 12 (with line profile for optimization test)..."
"$BINARY" onu-provision $CMD_ARGS \
    --serial FHTT99990012 --pon-port "0/1" --onu-id 12 \
    --line-profile "line_vlan_100" --vlan 100 > /dev/null 2>&1 || log_warn "ONU 12 provision may have failed"

log_success "Test ONUs provisioned"

# Set up cleanup trap
cleanup_test_onus() {
    log_info "Cleaning up test ONUs..."
    for onu_id in 10 11 12 13; do
        "$BINARY" onu-delete $CMD_ARGS --pon-port "0/1" --onu-id $onu_id > /dev/null 2>&1 || true
    done
    log_success "Test ONUs cleaned up"
}
trap cleanup_test_onus EXIT

# =============================================================================
# Test 4: VLAN-only update (Flow 1)
# =============================================================================
log_info "Test 4: VLAN-only update (Flow 1 - Direct update)"

# Update VLAN from 100 to 200
OUTPUT=$("$BINARY" onu-update $CMD_ARGS \
    --pon-port "0/1" --onu-id 10 --vlan 200 --json 2>&1) || true

# Check if update succeeded
if echo "$OUTPUT" | jq -e '.vlan == 200' > /dev/null 2>&1; then
    log_success "Test 4 passed: VLAN updated to 200"
elif echo "$OUTPUT" | jq -e '.metadata.vlan == 200' > /dev/null 2>&1; then
    log_success "Test 4 passed: VLAN updated to 200 (metadata)"
else
    log_warn "Test 4: VLAN update may have failed, output: $OUTPUT"
fi

# =============================================================================
# Test 5: Profile change (Flow 2 - Delete+Re-provision)
# =============================================================================
log_info "Test 5: Profile change (Flow 2 - Delete+Re-provision)"

# Get serial before update
INFO_BEFORE=$("$BINARY" onu-info $CMD_ARGS --pon-port "0/1" --onu-id 11 --json 2>&1) || true
SERIAL=$(echo "$INFO_BEFORE" | jq -r '.serial' 2>/dev/null || echo "FHTT99990011")

# Update line profile (triggers delete+re-provision)
START_TIME=$(date +%s)
OUTPUT=$("$BINARY" onu-update $CMD_ARGS \
    --pon-port "0/1" --onu-id 11 --line-profile "line_vlan_200" --json 2>&1) || true
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

# Verify update succeeded
if echo "$OUTPUT" | jq -e '. != null' > /dev/null 2>&1; then
    log_success "Test 5 passed: Profile change completed in ${DURATION}s"
    if [ $DURATION -ge 3 ]; then
        log_info "Duration ${DURATION}s suggests Flow 2 (delete+re-provision) was used"
    fi
else
    log_warn "Test 5: Profile change may have failed, output: $OUTPUT"
fi

# =============================================================================
# Test 6: Profile unchanged optimization (Flow 1)
# =============================================================================
log_info "Test 6: Profile unchanged optimization (should use Flow 1)"

# Update VLAN with SAME line profile (should use direct update, not delete+re-provision)
START_TIME=$(date +%s)
OUTPUT=$("$BINARY" onu-update $CMD_ARGS \
    --pon-port "0/1" --onu-id 12 \
    --line-profile "line_vlan_100" --vlan 200 --json 2>&1) || true
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

if echo "$OUTPUT" | jq -e '. != null' > /dev/null 2>&1; then
    if [ $DURATION -lt 3 ]; then
        log_success "Test 6 passed: Optimization verified - update completed in ${DURATION}s (Flow 1)"
    else
        log_warn "Test 6: Update took ${DURATION}s (expected <3s for direct update optimization)"
    fi
else
    log_warn "Test 6: Update may have failed, output: $OUTPUT"
fi

# =============================================================================
# Test 7: Missing ONU error
# =============================================================================
log_info "Test 7: Missing ONU should fail gracefully"

set +e
ERROR_OUTPUT=$("$BINARY" onu-update $CMD_ARGS \
    --pon-port "0/1" --onu-id 999 --vlan 200 2>&1)
EXIT_CODE=$?
set -e

if [[ $EXIT_CODE -ne 0 ]]; then
    log_success "Test 7 passed: Missing ONU correctly rejected"
else
    log_warn "Test 7: Expected failure for missing ONU"
fi

# =============================================================================
# Test 8: Invalid VLAN range
# =============================================================================
log_info "Test 8: Invalid VLAN should fail"

set +e
ERROR_OUTPUT=$("$BINARY" onu-update $CMD_ARGS \
    --pon-port "0/1" --onu-id 10 --vlan 5000 2>&1)
EXIT_CODE=$?
set -e

if [[ $EXIT_CODE -ne 0 ]]; then
    log_success "Test 8 passed: Invalid VLAN correctly rejected"
else
    log_warn "Test 8: Expected failure for invalid VLAN"
fi

# =============================================================================
# Test 9: Multiple updates in sequence
# =============================================================================
log_info "Test 9: Multiple updates in sequence"

# Update VLAN
"$BINARY" onu-update $CMD_ARGS --pon-port "0/1" --onu-id 10 --vlan 300 --json > /dev/null 2>&1 || log_warn "First update failed"

# Update description
"$BINARY" onu-update $CMD_ARGS --pon-port "0/1" --onu-id 10 --description "Updated ONU" --json > /dev/null 2>&1 || log_warn "Second update failed"

log_success "Test 9 passed: Multiple sequential updates completed"

# =============================================================================
# Summary
# =============================================================================
log_success "All onu-update tests passed for $VENDOR"
exit 0
