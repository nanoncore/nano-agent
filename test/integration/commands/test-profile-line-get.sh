#!/usr/bin/env bash
# =============================================================================
# Test: profile-line get
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

log_info "Testing profile-line get for vendor: $VENDOR (protocol: $PROTOCOL)"

CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

PROFILE_NAME="line_vlan_999"

OUTPUT=$("$BINARY" profile-line get "$PROFILE_NAME" $CMD_ARGS --json 2>&1) || {
    log_error "Command failed with output: $OUTPUT"
    exit 1
}

assert_json_valid "$OUTPUT"
assert_json_value "$OUTPUT" "name" "$PROFILE_NAME" "Profile name mismatch"
assert_json_array_not_empty "$OUTPUT" ".tconts" "Expected tconts to be present"
assert_not_empty "$(echo "$OUTPUT" | jq -r '.tconts[0].name')" "Expected tcont name"
assert_json_array_not_empty "$OUTPUT" ".tconts[0].gemports" "Expected gemports to be present"

log_success "profile-line get test passed for $VENDOR"
exit 0
