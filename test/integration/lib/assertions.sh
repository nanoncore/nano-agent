#!/usr/bin/env bash
# Test assertion helpers for nano-agent integration tests

# Source common utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

# Assert that a string contains a substring
assert_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-Expected to find '$needle' in output}"

    if [[ "$haystack" == *"$needle"* ]]; then
        return 0
    fi

    log_error "$msg"
    log_error "Output (first 500 chars): ${haystack:0:500}"
    return 1
}

# Assert that a string does NOT contain a substring
assert_not_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-Did not expect to find '$needle' in output}"

    if [[ "$haystack" != *"$needle"* ]]; then
        return 0
    fi

    log_error "$msg"
    return 1
}

# Assert that two strings are equal
assert_equals() {
    local expected="$1"
    local actual="$2"
    local msg="${3:-Values do not match}"

    if [[ "$expected" == "$actual" ]]; then
        return 0
    fi

    log_error "$msg"
    log_error "Expected: '$expected'"
    log_error "Actual:   '$actual'"
    return 1
}

# Assert that two strings are NOT equal
assert_not_equals() {
    local expected="$1"
    local actual="$2"
    local msg="${3:-Values should not match}"

    if [[ "$expected" != "$actual" ]]; then
        return 0
    fi

    log_error "$msg"
    log_error "Both values are: '$expected'"
    return 1
}

# Assert that a value is not empty
assert_not_empty() {
    local value="$1"
    local msg="${2:-Value should not be empty}"

    if [[ -n "$value" ]]; then
        return 0
    fi

    log_error "$msg"
    return 1
}

# Assert that a string is valid JSON
assert_json_valid() {
    local json="$1"
    local msg="${2:-Invalid JSON}"

    if echo "$json" | jq . > /dev/null 2>&1; then
        return 0
    fi

    log_error "$msg"
    log_error "Content (first 200 chars): ${json:0:200}"
    return 1
}

# Assert that JSON has a specific key
assert_json_has_key() {
    local json="$1"
    local key="$2"
    local msg="${3:-JSON missing key: $key}"

    if [[ $(echo "$json" | jq "has(\"$key\")") == "true" ]]; then
        return 0
    fi

    log_error "$msg"
    return 1
}

# Assert that JSON key has a specific value
assert_json_value() {
    local json="$1"
    local key="$2"
    local expected="$3"
    local msg="${4:-JSON key '$key' has unexpected value}"

    local actual
    actual=$(echo "$json" | jq -r ".$key")

    if [[ "$actual" == "$expected" ]]; then
        return 0
    fi

    log_error "$msg"
    log_error "Expected: '$expected'"
    log_error "Actual:   '$actual'"
    return 1
}

# Assert that JSON array is not empty
assert_json_array_not_empty() {
    local json="$1"
    local key="${2:-.}"
    local msg="${3:-JSON array should not be empty}"

    local length
    length=$(echo "$json" | jq "$key | length")

    if [[ "$length" -gt 0 ]]; then
        return 0
    fi

    log_error "$msg"
    return 1
}

# Assert exit code
assert_exit_code() {
    local expected="$1"
    local actual="$2"
    local msg="${3:-Unexpected exit code}"

    if [[ "$expected" == "$actual" ]]; then
        return 0
    fi

    log_error "$msg"
    log_error "Expected exit code: $expected"
    log_error "Actual exit code:   $actual"
    return 1
}

# Assert command succeeds (exit code 0)
assert_success() {
    local exit_code="$1"
    local msg="${2:-Command should succeed}"

    assert_exit_code 0 "$exit_code" "$msg"
}

# Assert command fails (exit code non-zero)
assert_failure() {
    local exit_code="$1"
    local msg="${2:-Command should fail}"

    if [[ "$exit_code" -ne 0 ]]; then
        return 0
    fi

    log_error "$msg"
    log_error "Expected non-zero exit code, got 0"
    return 1
}

# Assert a file exists
assert_file_exists() {
    local file="$1"
    local msg="${2:-File should exist: $file}"

    if [[ -f "$file" ]]; then
        return 0
    fi

    log_error "$msg"
    return 1
}

# Assert output matches a regex pattern
assert_matches() {
    local value="$1"
    local pattern="$2"
    local msg="${3:-Value does not match pattern}"

    if [[ "$value" =~ $pattern ]]; then
        return 0
    fi

    log_error "$msg"
    log_error "Pattern: $pattern"
    log_error "Value:   $value"
    return 1
}

# Assert a numeric value is greater than threshold
assert_greater_than() {
    local value="$1"
    local threshold="$2"
    local msg="${3:-Value should be greater than $threshold}"

    if (( $(echo "$value > $threshold" | bc -l) )); then
        return 0
    fi

    log_error "$msg"
    log_error "Value: $value, Threshold: $threshold"
    return 1
}

# Assert a numeric value is within range
assert_in_range() {
    local value="$1"
    local min="$2"
    local max="$3"
    local msg="${4:-Value should be in range [$min, $max]}"

    if (( $(echo "$value >= $min && $value <= $max" | bc -l) )); then
        return 0
    fi

    log_error "$msg"
    log_error "Value: $value, Range: [$min, $max]"
    return 1
}
