#!/usr/bin/env bash
# Common utilities for nano-agent integration tests

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $*"
}

log_section() {
    echo -e "\n${CYAN}========== $* ==========${NC}"
}

log_test_start() {
    echo -e "${BLUE}[TEST]${NC} Starting: $1"
}

log_test_pass() {
    local name="$1"
    local duration="${2:-0}"
    echo -e "${GREEN}[PASS]${NC} $name (${duration}s)"
}

log_test_fail() {
    local name="$1"
    local reason="${2:-unknown error}"
    echo -e "${RED}[FAIL]${NC} $name - $reason"
}

log_test_skip() {
    local name="$1"
    local reason="${2:-skipped}"
    echo -e "${YELLOW}[SKIP]${NC} $name - $reason"
}

# Initialize results file
init_results() {
    local results_file="$1"
    cat > "$results_file" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "tests": [],
  "summary": {}
}
EOF
}

# Record a test result to JSON file
record_result() {
    local results_file="$1"
    local test_name="$2"
    local status="$3"
    local duration="$4"
    local error_msg="${5:-}"
    local output_file="${6:-}"

    # Get output sample if file exists
    local output_sample=""
    if [[ -n "$output_file" && -f "$output_file" ]]; then
        # Get first 100 lines, base64 encode for safe JSON
        output_sample=$(head -100 "$output_file" 2>/dev/null | base64 | tr -d '\n')
    fi

    # Create result JSON
    local result_json
    result_json=$(jq -n \
        --arg cmd "$test_name" \
        --arg status "$status" \
        --argjson duration "$duration" \
        --arg error "$error_msg" \
        --arg output "$output_sample" \
        '{command: $cmd, status: $status, duration: $duration, error: $error, output_sample: $output}')

    # Append to results file
    local tmp_file
    tmp_file=$(mktemp)
    jq ".tests += [$result_json]" "$results_file" > "$tmp_file"
    mv "$tmp_file" "$results_file"
}

# Finalize results with summary
finalize_results() {
    local results_file="$1"
    local total="$2"
    local passed="$3"
    local failed="$4"
    local skipped="$5"

    local pass_rate=0
    if [[ $total -gt 0 ]]; then
        pass_rate=$(echo "scale=0; ($passed * 100) / $total" | bc)
    fi

    local tmp_file
    tmp_file=$(mktemp)
    jq --argjson total "$total" \
       --argjson passed "$passed" \
       --argjson failed "$failed" \
       --argjson skipped "$skipped" \
       --argjson rate "$pass_rate" \
       '.summary = {total: $total, passed: $passed, failed: $failed, skipped: $skipped, pass_rate: $rate}' \
       "$results_file" > "$tmp_file"
    mv "$tmp_file" "$results_file"
}

# Wait for a service to be healthy
wait_for_health() {
    local url="$1"
    local max_attempts="${2:-30}"
    local interval="${3:-2}"

    for ((i=1; i<=max_attempts; i++)); do
        if curl -sf "$url" > /dev/null 2>&1; then
            log_success "Service healthy at $url"
            return 0
        fi
        log_info "Waiting for $url... ($i/$max_attempts)"
        sleep "$interval"
    done

    log_error "Service failed to become healthy: $url"
    return 1
}

# Get vendor-specific SNMP connection arguments
get_snmp_args() {
    local vendor="$1"
    local address="${OLT_ADDRESS:-localhost}"

    case "$vendor" in
        vsol)
            echo "--address $address --port ${VSOL_SNMP_PORT:-161} --protocol snmp --community ${SNMP_COMMUNITY:-public}"
            ;;
        huawei)
            echo "--address $address --port ${HUAWEI_SNMP_PORT:-162} --protocol snmp --community ${SNMP_COMMUNITY:-public}"
            ;;
        *)
            log_error "Unknown vendor: $vendor"
            return 1
            ;;
    esac
}

# Get vendor-specific CLI (SSH) connection arguments
get_cli_args() {
    local vendor="$1"
    local address="${OLT_ADDRESS:-localhost}"

    case "$vendor" in
        vsol)
            echo "--address $address --port ${VSOL_SSH_PORT:-2222} --protocol cli --username ${SSH_USERNAME:-admin} --password ${SSH_PASSWORD:-admin}"
            ;;
        huawei)
            echo "--address $address --port ${HUAWEI_SSH_PORT:-2223} --protocol cli --username ${SSH_USERNAME:-admin} --password ${SSH_PASSWORD:-admin}"
            ;;
        *)
            log_error "Unknown vendor: $vendor"
            return 1
            ;;
    esac
}

# Get connection arguments - use CLI for write ops, SNMP for read ops
get_connection_args() {
    local vendor="$1"
    local cmd_type="${2:-read}"

    if [[ "$cmd_type" == "write" || "$cmd_type" == "cli" ]]; then
        get_cli_args "$vendor"
    else
        get_snmp_args "$vendor"
    fi
}

# Get common auth arguments (for backwards compatibility)
get_auth_args() {
    echo ""
}

# Measure execution time
measure_time() {
    local start_time end_time duration
    start_time=$(date +%s.%N)
    "$@"
    local exit_code=$?
    end_time=$(date +%s.%N)
    duration=$(echo "$end_time - $start_time" | bc)
    echo "$duration"
    return $exit_code
}
