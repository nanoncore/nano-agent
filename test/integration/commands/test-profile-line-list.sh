#!/usr/bin/env bash
# =============================================================================
# Test: profile-line list
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

log_info "Testing profile-line list for vendor: $VENDOR (protocol: $PROTOCOL)"

CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

OUTPUT=$("$BINARY" profile-line list $CMD_ARGS --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

assert_json_valid "$OUTPUT"
JSON_TYPE=$(echo "$OUTPUT" | jq -r 'type')
assert_equals "array" "$JSON_TYPE" "Expected JSON array output"
assert_json_array_not_empty "$OUTPUT" "." "Expected profile list to be non-empty"
assert_not_empty "$(echo "$OUTPUT" | jq -r '.[0].name')" "Expected first profile to have a name"
assert_not_empty "$(echo "$OUTPUT" | jq -r '.[0].id')" "Expected first profile to have an id"

log_success "profile-line list test passed for $VENDOR"
exit 0
