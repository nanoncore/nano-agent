#!/usr/bin/env bash
# =============================================================================
# Test: port-list command
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

log_info "Testing port-list for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

# =============================================================================
# Test 1: JSON output format
# =============================================================================
log_info "Test 1: JSON output format"

OUTPUT=$("$BINARY" port-list $CMD_ARGS --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

# Validate JSON structure
assert_json_valid "$OUTPUT"

# Check it's an array
IS_ARRAY=$(echo "$OUTPUT" | jq 'type == "array"')
assert_equals "true" "$IS_ARRAY" "Output should be a JSON array"

# Get count of ports
PORT_COUNT=$(echo "$OUTPUT" | jq 'length')
log_info "Found $PORT_COUNT PON ports"

# Should have at least some ports
assert_greater_than "$PORT_COUNT" 0 "Should have at least one PON port"

# Validate first port structure
if [[ "$PORT_COUNT" -gt 0 ]]; then
    FIRST_PORT=$(echo "$OUTPUT" | jq '.[0]')

    # Check for common port fields
    for field in "name" "index"; do
        if echo "$FIRST_PORT" | jq -e "has(\"$field\")" > /dev/null 2>&1; then
            log_info "Field '$field' present"
        fi
    done
fi

log_info "Test 1 passed: JSON output is valid"

# =============================================================================
# Test 2: Table output
# =============================================================================
log_info "Test 2: Table output format"

TABLE_OUTPUT=$("$BINARY" port-list $CMD_ARGS 2>&1) || {
    log_error "Table output command failed: $TABLE_OUTPUT"
    exit 1
}

assert_not_empty "$TABLE_OUTPUT" "Table output should not be empty"

log_info "Test 2 passed: Table output works"

# =============================================================================
# Summary
# =============================================================================
log_success "All port-list tests passed for $VENDOR"
exit 0
