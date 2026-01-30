#!/usr/bin/env bash
# =============================================================================
# Test: discover command (find unprovisioned ONUs)
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

log_info "Testing discover for vendor: $VENDOR (protocol: $PROTOCOL)"

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

OUTPUT=$("$BINARY" discover $CMD_ARGS --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

# Validate JSON structure
assert_json_valid "$OUTPUT"

# Should be an array (could be empty if no unprovisioned ONUs)
IS_ARRAY=$(echo "$OUTPUT" | jq 'type == "array"')
assert_equals "true" "$IS_ARRAY" "Output should be a JSON array"

# Get count
DISCOVERED_COUNT=$(echo "$OUTPUT" | jq 'length')
log_info "Found $DISCOVERED_COUNT unprovisioned ONUs"

# If there are discovered ONUs, validate structure
if [[ "$DISCOVERED_COUNT" -gt 0 ]]; then
    FIRST_ONU=$(echo "$OUTPUT" | jq '.[0]')

    # Check for serial number field
    if echo "$FIRST_ONU" | jq -e 'has("serial")' > /dev/null 2>&1; then
        SERIAL=$(echo "$FIRST_ONU" | jq -r '.serial')
        log_info "First discovered ONU serial: $SERIAL"
    fi
fi

log_info "Test 1 passed: JSON output is valid"

# =============================================================================
# Test 2: Table output
# =============================================================================
log_info "Test 2: Table output format"

TABLE_OUTPUT=$("$BINARY" discover $CMD_ARGS 2>&1) || {
    log_error "Table output command failed: $TABLE_OUTPUT"
    exit 1
}

# Output might indicate no ONUs found, which is valid
assert_not_empty "$TABLE_OUTPUT" "Output should not be empty"

log_info "Test 2 passed: Table output works"

# =============================================================================
# Summary
# =============================================================================
log_success "All discover tests passed for $VENDOR"
exit 0
