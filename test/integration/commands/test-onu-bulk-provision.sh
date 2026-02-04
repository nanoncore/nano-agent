#!/usr/bin/env bash
# =============================================================================
# Test: onu-bulk-provision command
# =============================================================================
set -euo pipefail

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

log_info "Testing onu-bulk-provision for vendor: $VENDOR (protocol: $PROTOCOL)"

# Build CLI command arguments
CMD_ARGS="--vendor $VENDOR --address $ADDRESS --port $PORT --protocol $PROTOCOL"
if [[ -n "$USERNAME" ]]; then
    CMD_ARGS="$CMD_ARGS --username $USERNAME --password $PASSWORD"
fi

if [[ "$VENDOR" != "vsol" ]]; then
    log_warn "Bulk provision test skipped: only validated for vsol"
    exit 0
fi

TMP_CSV=$(mktemp)
trap 'rm -f "$TMP_CSV"' EXIT

# Default simulator targets
PON1="${VSOL_BULK_PON1:-0/1}"
PON2="${VSOL_BULK_PON2:-0/2}"
ONU1_ID="${VSOL_BULK_ONU1_ID:-11}"
ONU2_ID="${VSOL_BULK_ONU2_ID:-12}"
ONU3_ID="${VSOL_BULK_ONU3_ID:-11}"
SER1="${VSOL_BULK_SERIAL1:-FHTT99990001}"
SER2="${VSOL_BULK_SERIAL2:-FHTT99990002}"
SER3="${VSOL_BULK_SERIAL3:-FHTT99990003}"
VLAN="${VSOL_BULK_VLAN:-702}"
PROFILE="${VSOL_BULK_PROFILE:-AN5506-04-F1}"

cat > "$TMP_CSV" <<EOF
serial,pon_port,onu_id,vlan,line_profile,service_profile,bandwidth_up,bandwidth_down
$SER1,$PON1,$ONU1_ID,$VLAN,$PROFILE,,100000,100000
$SER2,$PON1,$ONU2_ID,$VLAN,$PROFILE,,100000,100000
$SER3,$PON2,$ONU3_ID,$VLAN,$PROFILE,,100000,100000
EOF

set +e
OUTPUT=$("$BINARY" onu-bulk-provision $CMD_ARGS --csv "$TMP_CSV" 2>&1)
EXIT_CODE=$?
set -e

if [[ $EXIT_CODE -ne 0 ]]; then
    log_error "Bulk provision command failed"
    log_error "$OUTPUT"
    exit 1
fi

# Verify via SNMP (best-effort for simulator)
if command -v snmpwalk &> /dev/null && [[ -n "$COMMUNITY" ]]; then
    for entry in \
        "$SER1,$PON1,$ONU1_ID" \
        "$SER2,$PON1,$ONU2_ID" \
        "$SER3,$PON2,$ONU3_ID"
    do
        IFS=',' read -r SERIAL PORT ONU_ID <<< "$entry"
        if [[ "$PORT" =~ ^0/([0-9]+)$ ]]; then
            PON_INDEX="${BASH_REMATCH[1]}"
            OID="1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5.${PON_INDEX}.${ONU_ID}"
            VALUE=$(snmpwalk -v2c -c "$COMMUNITY" "$ADDRESS" "$OID" 2>/dev/null || true)
            if ! echo "$VALUE" | grep -qi "$SERIAL"; then
                log_error "Serial not found for $PORT onu $ONU_ID"
                log_error "$VALUE"
                exit 1
            fi
        else
            log_warn "Skipped SNMP verification for $PORT (unsupported format)"
        fi
    done
    log_info "SNMP verification passed for bulk provisioned ONUs"
else
    log_warn "SNMP verification skipped (snmpwalk unavailable or no community)"
fi

log_success "All onu-bulk-provision tests passed for $VENDOR"
exit 0
