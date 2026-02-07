#!/usr/bin/env bash
# =============================================================================
# Test: onu-list command
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

log_info "Testing onu-list for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

# =============================================================================
# Test 1: Basic execution with JSON output
# =============================================================================
log_info "Test 1: JSON output format"

OUTPUT=$("$BINARY" onu-list $CMD_ARGS --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

# Validate JSON structure (should be an array)
assert_json_valid "$OUTPUT"

# Check it's an array
IS_ARRAY=$(echo "$OUTPUT" | jq 'type == "array"')
assert_equals "true" "$IS_ARRAY" "Output should be a JSON array"

# Get count of ONUs
ONU_COUNT=$(echo "$OUTPUT" | jq 'length')
log_info "Found $ONU_COUNT ONUs"

# If there are ONUs, validate their structure
if [[ "$ONU_COUNT" -gt 0 ]]; then
    # Check first ONU has expected fields
    FIRST_ONU=$(echo "$OUTPUT" | jq '.[0]')

    # Common fields that should exist
    for field in "serial" "pon_port"; do
        if echo "$FIRST_ONU" | jq -e "has(\"$field\")" > /dev/null 2>&1; then
            log_info "Field '$field' present"
        fi
    done
fi

log_info "Test 1 passed: JSON output is valid"

# If V-SOL SNMP is used, ensure at least one ONU reports line_profile
if [[ "$VENDOR" == "vsol" && "$PROTOCOL" == "snmp" ]]; then
    LINE_PROFILE_COUNT=$(echo "$OUTPUT" | jq '[.[] | select(.line_profile != null and .line_profile != "")] | length')
    if [[ "$LINE_PROFILE_COUNT" -eq 0 ]]; then
        log_error "Expected at least one ONU with line_profile via SNMP"
        exit 1
    fi
    log_info "Found $LINE_PROFILE_COUNT ONUs with line_profile"
fi

# =============================================================================
# Test 2: Table output format
# =============================================================================
log_info "Test 2: Table output format"

TABLE_OUTPUT=$("$BINARY" onu-list $CMD_ARGS 2>&1) || {
    log_error "Table output command failed: $TABLE_OUTPUT"
    exit 1
}

assert_not_empty "$TABLE_OUTPUT" "Table output should not be empty"

log_info "Test 2 passed: Table output works"

# =============================================================================
# Summary
# =============================================================================
log_success "All onu-list tests passed for $VENDOR"
exit 0
