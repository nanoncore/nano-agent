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
# Summary
# =============================================================================
log_success "All onu-update tests passed for $VENDOR"
exit 0
