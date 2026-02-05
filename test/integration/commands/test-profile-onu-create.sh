#!/usr/bin/env bash
# =============================================================================
# Test: profile-onu create
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

log_info "Testing profile-onu create for vendor: $VENDOR (protocol: $PROTOCOL)"

CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi
if [[ -n "$COMMUNITY" ]]; then
    CMD_ARGS="$CMD_ARGS --community $COMMUNITY"
fi

PROFILE_NAME="cmdex_profile_$(date +%s)"
DESCRIPTION="cmdex profile"

CREATE_OUTPUT=$(
    "$BINARY" profile-onu create "$PROFILE_NAME" $CMD_ARGS \
      --port-eth 1 --port-iphost 2 --tcont-num 8 --gemport-num 32 \
      --service-ability n:1 --description "$DESCRIPTION" 2>&1
) || {
    log_error "Create failed with output: $CREATE_OUTPUT"
    exit 1
}

assert_contains "$CREATE_OUTPUT" "Profile created" "Expected create success message"

LIST_OUTPUT=$("$BINARY" profile-onu list $CMD_ARGS --json 2>&1) || {
    log_error "List failed with output: $LIST_OUTPUT"
    exit 1
}

assert_json_valid "$LIST_OUTPUT"
MATCH_COUNT=$(echo "$LIST_OUTPUT" | jq --arg name "$PROFILE_NAME" '[.[] | select(.name == $name)] | length')
assert_equals "1" "$MATCH_COUNT" "Expected created profile to appear in list"

DELETE_OUTPUT=$("$BINARY" profile-onu delete "$PROFILE_NAME" $CMD_ARGS 2>&1) || {
    log_error "Cleanup delete failed with output: $DELETE_OUTPUT"
    exit 1
}

assert_contains "$DELETE_OUTPUT" "Profile delete requested" "Expected delete success message"

log_success "profile-onu create test passed for $VENDOR"
exit 0
