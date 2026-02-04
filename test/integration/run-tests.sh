#!/usr/bin/env bash
# =============================================================================
# nano-agent Integration Test Runner
# =============================================================================
# Usage:
#   ./run-tests.sh --vendor vsol --output-dir ./results
#   ./run-tests.sh --vendor huawei --commands "olt-status,onu-list"
#   ./run-tests.sh --vendor all --timeout 120
# =============================================================================

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

# =============================================================================
# Default Configuration
# =============================================================================

VENDOR="all"
OUTPUT_DIR="$SCRIPT_DIR/results"
COMMANDS="all"
TIMEOUT=60
NANO_AGENT_BINARY="${NANO_AGENT_BINARY:-}"
SKIP_WRITE_TESTS="${SKIP_WRITE_TESTS:-false}"

# OLT Connection defaults
export OLT_ADDRESS="${OLT_ADDRESS:-localhost}"
export VSOL_SSH_PORT="${VSOL_SSH_PORT:-2222}"
export VSOL_SNMP_PORT="${VSOL_SNMP_PORT:-161}"
export HUAWEI_SSH_PORT="${HUAWEI_SSH_PORT:-2223}"
export HUAWEI_SNMP_PORT="${HUAWEI_SNMP_PORT:-162}"
export SSH_USERNAME="${SSH_USERNAME:-admin}"
export SSH_PASSWORD="${SSH_PASSWORD:-admin}"
export SNMP_COMMUNITY="${SNMP_COMMUNITY:-public}"

# =============================================================================
# Parse Arguments
# =============================================================================

while [[ $# -gt 0 ]]; do
    case $1 in
        --vendor)
            VENDOR="$2"
            shift 2
            ;;
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --commands)
            COMMANDS="$2"
            shift 2
            ;;
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        --binary)
            NANO_AGENT_BINARY="$2"
            shift 2
            ;;
        --skip-write)
            SKIP_WRITE_TESTS="true"
            shift
            ;;
        --help|-h)
            cat <<EOF
nano-agent Integration Test Runner

Usage: $0 [OPTIONS]

Options:
  --vendor <name>       Vendor to test: vsol, huawei, or all (default: all)
  --output-dir <path>   Output directory for results (default: ./results)
  --commands <list>     Comma-separated list of commands to test, or "all"
  --timeout <seconds>   Command timeout in seconds (default: 60)
  --binary <path>       Path to nano-agent binary
  --skip-write          Skip write operation tests (provision, delete, etc.)
  --help, -h            Show this help message

Environment Variables:
  NANO_AGENT_BINARY     Path to nano-agent binary
  OLT_ADDRESS           OLT address (default: localhost)
  VSOL_SSH_PORT         V-SOL SSH port (default: 2222)
  VSOL_SNMP_PORT        V-SOL SNMP port (default: 161)
  HUAWEI_SSH_PORT       Huawei SSH port (default: 2223)
  HUAWEI_SNMP_PORT      Huawei SNMP port (default: 162)
  SSH_USERNAME          SSH username (default: admin)
  SSH_PASSWORD          SSH password (default: admin)
  SNMP_COMMUNITY        SNMP community (default: public)

Examples:
  $0 --vendor vsol
  $0 --vendor huawei --commands "olt-status,onu-list"
  $0 --vendor all --skip-write --output-dir ./my-results
EOF
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# =============================================================================
# Validation
# =============================================================================

# Find nano-agent binary
if [[ -z "$NANO_AGENT_BINARY" ]]; then
    # Try common locations
    for path in "./nano-agent" "../../nano-agent" "../nano-agent"; do
        if [[ -x "$path" ]]; then
            NANO_AGENT_BINARY="$path"
            break
        fi
    done
    # Try which as fallback
    if [[ -z "$NANO_AGENT_BINARY" ]]; then
        NANO_AGENT_BINARY=$(which nano-agent 2>/dev/null || true)
    fi
fi

if [[ -z "$NANO_AGENT_BINARY" || ! -x "$NANO_AGENT_BINARY" ]]; then
    log_error "nano-agent binary not found. Set NANO_AGENT_BINARY or use --binary"
    exit 1
fi

log_info "Using nano-agent binary: $NANO_AGENT_BINARY"

# =============================================================================
# Setup
# =============================================================================

mkdir -p "$OUTPUT_DIR"
RESULTS_FILE="$OUTPUT_DIR/results.json"
init_results "$RESULTS_FILE"

# Command definitions: command_name:type (read/write)
# Using simple array instead of associative array for bash 3.x compatibility
READ_COMMANDS="olt-status olt-alarms olt-health-check onu-list onu-info diagnose port-list port-power service-port-list discover vlan-list vlan-get"
WRITE_COMMANDS="onu-provision onu-delete onu-suspend onu-resume onu-reboot onu-bulk-provision port-enable port-disable vlan-create vlan-delete service-port-add service-port-delete"

# Function to check if command is a write command
is_write_command() {
    local cmd="$1"
    for w in $WRITE_COMMANDS; do
        if [[ "$cmd" == "$w" ]]; then
            return 0
        fi
    done
    return 1
}

# Function to check if command requires CLI for specific vendor
requires_cli() {
    local vendor="$1"
    local cmd="$2"
    # V-SOL now supports SNMP for read operations (onu-list, olt-status, port-list, onu-info)
    # Only write operations (provision, delete, reboot), discover, and diagnostics require CLI
    if [[ "$vendor" == "vsol" ]]; then
        case "$cmd" in
            provision|delete|reboot|configure|discover|diagnose|vlan-list|vlan-get|vlan-create|vlan-delete|service-port-list|service-port-add|service-port-delete|olt-alarms)
                return 0  # These still require CLI
                ;;
            *)
                return 1  # Read operations can use SNMP
                ;;
        esac
    fi
    # Huawei discover requires CLI
    if [[ "$vendor" == "huawei" && ( "$cmd" == "discover" || "$cmd" == "service-port-list" ) ]]; then
        return 0
    fi
    return 1
}

is_unsupported_command() {
    local vendor="$1"
    local cmd="$2"

    if [[ "$vendor" == "huawei" ]]; then
        case "$cmd" in
            olt-status|olt-alarms|olt-health-check|onu-list|onu-info|port-list|port-power|service-port-list|discover|vlan-list|vlan-get|port-enable|port-disable|vlan-create|vlan-delete|service-port-add|service-port-delete)
                return 0
                ;;
            *)
                return 1
                ;;
        esac
    fi

    return 1
}

# Get all commands
ALL_COMMANDS="$READ_COMMANDS $WRITE_COMMANDS"

# Determine which vendors to test
case "$VENDOR" in
    all)
        VENDORS_TO_TEST="vsol huawei"
        ;;
    vsol|huawei)
        VENDORS_TO_TEST="$VENDOR"
        ;;
    *)
        log_error "Invalid vendor: $VENDOR (use vsol, huawei, or all)"
        exit 1
        ;;
esac

# =============================================================================
# Test Execution
# =============================================================================

TOTAL=0
PASSED=0
FAILED=0
SKIPPED=0

log_section "Starting Integration Tests"
log_info "Vendor(s): $VENDORS_TO_TEST"
log_info "Output directory: $OUTPUT_DIR"
log_info "Timeout: ${TIMEOUT}s"
log_info "Skip write tests: $SKIP_WRITE_TESTS"

for vendor in $VENDORS_TO_TEST; do
    log_section "Testing vendor: $vendor"

    for cmd in $ALL_COMMANDS; do
        # Filter by command list if specified
        if [[ "$COMMANDS" != "all" && ! ",$COMMANDS," == *",$cmd,"* ]]; then
            continue
        fi

        # Determine command type (read or write)
        # Some read commands may require CLI for specific vendors
        CMD_TYPE="read"
        if is_write_command "$cmd"; then
            CMD_TYPE="write"
        elif requires_cli "$vendor" "$cmd"; then
            CMD_TYPE="cli"
        fi

        # Skip write tests if requested
        if [[ "$SKIP_WRITE_TESTS" == "true" && "$CMD_TYPE" == "write" ]]; then
            log_test_skip "$vendor/$cmd" "write tests skipped"
            SKIPPED=$((SKIPPED + 1))
            TOTAL=$((TOTAL + 1))
            record_result "$RESULTS_FILE" "$vendor/$cmd" "skipped" 0 "Write tests skipped" ""
            continue
        fi

        TOTAL=$((TOTAL + 1))
        TEST_NAME="$vendor/$cmd"
        log_test_start "$TEST_NAME"

        if is_unsupported_command "$vendor" "$cmd"; then
            log_test_skip "$TEST_NAME" "unsupported for vendor"
            SKIPPED=$((SKIPPED + 1))
            record_result "$RESULTS_FILE" "$TEST_NAME" "skipped" 0 "Unsupported for vendor" ""
            continue
        fi

        # Check if test script exists
        TEST_SCRIPT="$SCRIPT_DIR/commands/test-${cmd}.sh"
        if [[ ! -f "$TEST_SCRIPT" ]]; then
            log_test_skip "$TEST_NAME" "no test script"
            SKIPPED=$((SKIPPED + 1))
            record_result "$RESULTS_FILE" "$TEST_NAME" "skipped" 0 "No test script found" ""
            continue
        fi

        # Get connection arguments based on command type
        CONN_ARGS=$(get_connection_args "$vendor" "$CMD_TYPE")

        # Prepare output files
        OUTPUT_FILE="$OUTPUT_DIR/${vendor}-${cmd}.log"
        STDERR_FILE="$OUTPUT_DIR/${vendor}-${cmd}.stderr"

        # Run test with timeout
        START_TIME=$(date +%s)

        set +e
        if command -v gtimeout &> /dev/null; then
            # macOS with coreutils
            gtimeout "$TIMEOUT" bash "$TEST_SCRIPT" \
                --binary "$NANO_AGENT_BINARY" \
                --vendor "$vendor" \
                --cmd-type "$CMD_TYPE" \
                $CONN_ARGS \
                > "$OUTPUT_FILE" 2> "$STDERR_FILE"
            EXIT_CODE=$?
        elif command -v timeout &> /dev/null; then
            # Linux
            timeout "$TIMEOUT" bash "$TEST_SCRIPT" \
                --binary "$NANO_AGENT_BINARY" \
                --vendor "$vendor" \
                --cmd-type "$CMD_TYPE" \
                $CONN_ARGS \
                > "$OUTPUT_FILE" 2> "$STDERR_FILE"
            EXIT_CODE=$?
        else
            # No timeout command available, run without timeout
            bash "$TEST_SCRIPT" \
                --binary "$NANO_AGENT_BINARY" \
                --vendor "$vendor" \
                --cmd-type "$CMD_TYPE" \
                $CONN_ARGS \
                > "$OUTPUT_FILE" 2> "$STDERR_FILE"
            EXIT_CODE=$?
        fi
        set -e

        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))

        # Evaluate result
        if [[ $EXIT_CODE -eq 0 ]]; then
            PASSED=$((PASSED + 1))
            log_test_pass "$TEST_NAME" "$DURATION"
            record_result "$RESULTS_FILE" "$TEST_NAME" "pass" "$DURATION" "" "$OUTPUT_FILE"
        elif [[ $EXIT_CODE -eq 124 ]]; then
            FAILED=$((FAILED + 1))
            log_test_fail "$TEST_NAME" "timeout after ${TIMEOUT}s"
            record_result "$RESULTS_FILE" "$TEST_NAME" "fail" "$DURATION" "Timeout after ${TIMEOUT}s" "$OUTPUT_FILE"
        else
            FAILED=$((FAILED + 1))
            ERROR_MSG=$(tail -5 "$STDERR_FILE" 2>/dev/null | tr '\n' ' ' || echo "Unknown error")
            log_test_fail "$TEST_NAME" "exit code $EXIT_CODE: $ERROR_MSG"
            record_result "$RESULTS_FILE" "$TEST_NAME" "fail" "$DURATION" "$ERROR_MSG" "$OUTPUT_FILE"
        fi
    done
done

# =============================================================================
# Finalize Results
# =============================================================================

finalize_results "$RESULTS_FILE" "$TOTAL" "$PASSED" "$FAILED" "$SKIPPED"

log_section "Test Summary"
log_info "Total:   $TOTAL"
log_info "Passed:  $PASSED"
log_info "Failed:  $FAILED"
log_info "Skipped: $SKIPPED"

if [[ $TOTAL -gt 0 ]]; then
    PASS_RATE=$((PASSED * 100 / TOTAL))
    log_info "Pass rate: ${PASS_RATE}%"
fi

log_info "Results saved to: $RESULTS_FILE"

# Exit with failure if any tests failed
if [[ $FAILED -gt 0 ]]; then
    log_error "Some tests failed"
    exit 1
fi

log_success "All tests passed!"
exit 0
