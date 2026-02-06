#!/usr/bin/env bash
# =============================================================================
# Test: profile-line create
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

log_info "Testing profile-line create for vendor: $VENDOR (protocol: $PROTOCOL)"

CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

PROFILE_NAME="cmdex_line_profile_$(date +%s)"

CREATE_OUTPUT=$(
    "$BINARY" profile-line create "$PROFILE_NAME" $CMD_ARGS \
      --tcont "id=1,name=tcont_1,dba=default" \
      --gemport "id=1,name=gemport_1,tcont=1,traffic-up=default,traffic-down=default" \
      --service "name=INTERNET,gemport=1,vlan=100,cos=0-7" \
      --service-port "id=1,gemport=1,uservlan=100,vlan=100" \
      --mvlan "vlans=200,201" 2>&1
) || {
    log_error "Create failed with output: $CREATE_OUTPUT"
    exit 1
}

assert_contains "$CREATE_OUTPUT" "Profile created" "Expected create success message"

GET_OUTPUT=$("$BINARY" profile-line get "$PROFILE_NAME" $CMD_ARGS --json 2>&1) || {
    log_error "Get failed with output: $GET_OUTPUT"
    exit 1
}

assert_json_valid "$GET_OUTPUT"
assert_json_value "$GET_OUTPUT" "name" "$PROFILE_NAME" "Profile name mismatch"
assert_json_array_not_empty "$GET_OUTPUT" ".tconts" "Expected tconts to be present"
assert_not_empty "$(echo "$GET_OUTPUT" | jq -r '.tconts[0].name')" "Expected tcont name"
assert_json_array_not_empty "$GET_OUTPUT" ".tconts[0].gemports" "Expected gemports to be present"

DELETE_OUTPUT=$("$BINARY" profile-line delete "$PROFILE_NAME" $CMD_ARGS 2>&1) || {
    log_error "Cleanup delete failed with output: $DELETE_OUTPUT"
    exit 1
}

assert_contains "$DELETE_OUTPUT" "Profile delete requested" "Expected delete success message"

log_success "profile-line create test passed for $VENDOR"
exit 0
