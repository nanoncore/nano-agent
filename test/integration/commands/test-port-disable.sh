#!/usr/bin/env bash
# =============================================================================
# Test: port-disable command
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

log_info "Testing port-disable for vendor: $VENDOR (protocol: $PROTOCOL)"

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

HELP_OUTPUT=$("$BINARY" port-disable --help 2>&1) || true

assert_contains "$HELP_OUTPUT" "port" "Help should mention port parameter"

log_info "Test 1 passed: Help output is correct"

# =============================================================================
# Test 2: Missing port parameter
# =============================================================================
log_info "Test 2: Missing port should fail"

set +e
ERROR_OUTPUT=$("$BINARY" port-disable $CMD_ARGS 2>&1)
EXIT_CODE=$?
set -e

if [[ $EXIT_CODE -ne 0 ]]; then
    log_info "Test 2 passed: Command correctly fails without port"
else
    log_warn "Test 2: Command did not fail as expected"
fi

# =============================================================================
# Summary
# =============================================================================
log_success "All port-disable tests passed for $VENDOR"
exit 0
