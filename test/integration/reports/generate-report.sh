#!/usr/bin/env bash
# =============================================================================
# Generate HTML and JSON summary reports from test results
# =============================================================================
# Usage:
#   ./generate-report.sh ./results
#   ./generate-report.sh ./results-vsol ./results-huawei --combined --output ./combined
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Defaults
INPUT_DIRS=()
OUTPUT_DIR=""
COMBINED=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --combined)
            COMBINED=true
            shift
            ;;
        --output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --help|-h)
            cat <<EOF
Generate test reports from integration test results.

Usage: $0 [OPTIONS] <input_dir> [input_dir2 ...]

Options:
  --combined        Merge multiple result directories
  --output <path>   Output directory for reports
  --help, -h        Show this help message

Examples:
  $0 ./results
  $0 ./results-vsol ./results-huawei --combined --output ./combined-report
EOF
            exit 0
            ;;
        *)
            INPUT_DIRS+=("$1")
            shift
            ;;
    esac
done

# Validate inputs
if [[ ${#INPUT_DIRS[@]} -eq 0 ]]; then
    echo "Error: No input directories specified"
    exit 1
fi

# Set default output dir
if [[ -z "$OUTPUT_DIR" ]]; then
    OUTPUT_DIR="${INPUT_DIRS[0]}"
fi

mkdir -p "$OUTPUT_DIR"

# =============================================================================
# Merge or copy results
# =============================================================================

if [[ "$COMBINED" == "true" && ${#INPUT_DIRS[@]} -gt 1 ]]; then
    echo "Merging results from ${#INPUT_DIRS[@]} directories..."

    MERGED_FILE="$OUTPUT_DIR/results.json"

    # Start with empty structure
    cat > "$MERGED_FILE" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "tests": [],
  "summary": {}
}
EOF

    # Merge all test results
    for dir in "${INPUT_DIRS[@]}"; do
        RESULTS_FILE="$dir/results.json"
        if [[ -f "$RESULTS_FILE" ]]; then
            echo "  Merging: $RESULTS_FILE"
            TMP_FILE=$(mktemp)
            jq -s '.[0].tests = (.[0].tests + .[1].tests) | .[0]' \
                "$MERGED_FILE" "$RESULTS_FILE" > "$TMP_FILE"
            mv "$TMP_FILE" "$MERGED_FILE"
        fi
    done

    INPUT_FILE="$MERGED_FILE"
else
    INPUT_FILE="${INPUT_DIRS[0]}/results.json"

    if [[ ! -f "$INPUT_FILE" ]]; then
        echo "Error: Results file not found: $INPUT_FILE"
        exit 1
    fi
fi

# =============================================================================
# Recalculate summary
# =============================================================================

echo "Calculating summary..."

TOTAL=$(jq '.tests | length' "$INPUT_FILE")
PASSED=$(jq '[.tests[] | select(.status == "pass")] | length' "$INPUT_FILE")
FAILED=$(jq '[.tests[] | select(.status == "fail")] | length' "$INPUT_FILE")
SKIPPED=$(jq '[.tests[] | select(.status == "skipped")] | length' "$INPUT_FILE")

PASS_RATE=0
if [[ $TOTAL -gt 0 ]]; then
    PASS_RATE=$(echo "scale=0; ($PASSED * 100) / $TOTAL" | bc)
fi

# Update summary in results file
TMP_FILE=$(mktemp)
jq --argjson total "$TOTAL" \
   --argjson passed "$PASSED" \
   --argjson failed "$FAILED" \
   --argjson skipped "$SKIPPED" \
   --argjson rate "$PASS_RATE" \
   '.summary = {total: $total, passed: $passed, failed: $failed, skipped: $skipped, pass_rate: $rate}' \
   "$INPUT_FILE" > "$TMP_FILE"
mv "$TMP_FILE" "$OUTPUT_DIR/summary.json"

echo "Summary: $PASSED/$TOTAL passed ($PASS_RATE%), $FAILED failed, $SKIPPED skipped"

# =============================================================================
# Generate HTML Report
# =============================================================================

echo "Generating HTML report..."

# Read the JSON data
JSON_DATA=$(cat "$OUTPUT_DIR/summary.json")

# Escape JSON for embedding in JavaScript
JSON_ESCAPED=$(echo "$JSON_DATA" | jq -c .)

cat > "$OUTPUT_DIR/report.html" <<'HTMLEOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>nano-agent Integration Test Report</title>
    <style>
        * { box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            margin: 0;
            padding: 20px;
            background: #f5f7fa;
            color: #333;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            padding: 30px;
            border-radius: 12px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        h1 {
            color: #1a1a2e;
            border-bottom: 3px solid #4361ee;
            padding-bottom: 15px;
            margin-top: 0;
        }
        .timestamp {
            color: #666;
            font-size: 14px;
            margin-bottom: 20px;
        }
        .summary {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 15px;
            margin: 25px 0;
        }
        .stat {
            padding: 20px;
            border-radius: 10px;
            text-align: center;
            transition: transform 0.2s;
        }
        .stat:hover { transform: translateY(-2px); }
        .stat-total { background: linear-gradient(135deg, #e3f2fd 0%, #bbdefb 100%); color: #1565c0; }
        .stat-pass { background: linear-gradient(135deg, #e8f5e9 0%, #c8e6c9 100%); color: #2e7d32; }
        .stat-fail { background: linear-gradient(135deg, #ffebee 0%, #ffcdd2 100%); color: #c62828; }
        .stat-skip { background: linear-gradient(135deg, #fff8e1 0%, #ffecb3 100%); color: #f57f17; }
        .stat-number { font-size: 42px; font-weight: 700; }
        .stat-label { font-size: 13px; text-transform: uppercase; letter-spacing: 1px; margin-top: 5px; }
        .progress-bar {
            height: 8px;
            background: #e0e0e0;
            border-radius: 4px;
            overflow: hidden;
            margin: 20px 0;
        }
        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #4caf50 0%, #8bc34a 100%);
            transition: width 0.5s ease;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        th, td {
            padding: 14px 16px;
            text-align: left;
            border-bottom: 1px solid #e0e0e0;
        }
        th {
            background: #f8f9fa;
            font-weight: 600;
            color: #555;
            text-transform: uppercase;
            font-size: 12px;
            letter-spacing: 0.5px;
        }
        tr:hover { background: #f8f9fa; }
        .status-pass { color: #2e7d32; font-weight: 600; }
        .status-fail { color: #c62828; font-weight: 600; }
        .status-skipped { color: #f57f17; font-weight: 600; }
        .duration { color: #666; font-family: monospace; }
        .error { color: #c62828; font-size: 12px; max-width: 400px; word-wrap: break-word; }
        .command { font-family: monospace; font-weight: 500; }
        .vendor-badge {
            display: inline-block;
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
        }
        .vendor-vsol { background: #e3f2fd; color: #1565c0; }
        .vendor-huawei { background: #fce4ec; color: #c2185b; }
        footer {
            margin-top: 30px;
            padding-top: 20px;
            border-top: 1px solid #e0e0e0;
            color: #888;
            font-size: 12px;
            text-align: center;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>nano-agent Integration Test Report</h1>
        <p class="timestamp">Generated: <span id="timestamp"></span></p>

        <div class="summary">
            <div class="stat stat-total">
                <div class="stat-number" id="total">-</div>
                <div class="stat-label">Total Tests</div>
            </div>
            <div class="stat stat-pass">
                <div class="stat-number" id="passed">-</div>
                <div class="stat-label">Passed</div>
            </div>
            <div class="stat stat-fail">
                <div class="stat-number" id="failed">-</div>
                <div class="stat-label">Failed</div>
            </div>
            <div class="stat stat-skip">
                <div class="stat-number" id="skipped">-</div>
                <div class="stat-label">Skipped</div>
            </div>
        </div>

        <div class="progress-bar">
            <div class="progress-fill" id="progress"></div>
        </div>
        <p style="text-align: center; color: #666;">
            Pass Rate: <strong id="pass-rate">-</strong>%
        </p>

        <table>
            <thead>
                <tr>
                    <th>Vendor</th>
                    <th>Command</th>
                    <th>Status</th>
                    <th>Duration</th>
                    <th>Error</th>
                </tr>
            </thead>
            <tbody id="results"></tbody>
        </table>

        <footer>
            Generated by nano-agent integration test framework
        </footer>
    </div>

    <script>
        const data = REPLACE_WITH_DATA;

        // Update summary
        document.getElementById('timestamp').textContent = data.timestamp || new Date().toISOString();
        document.getElementById('total').textContent = data.summary?.total || 0;
        document.getElementById('passed').textContent = data.summary?.passed || 0;
        document.getElementById('failed').textContent = data.summary?.failed || 0;
        document.getElementById('skipped').textContent = data.summary?.skipped || 0;

        const passRate = data.summary?.pass_rate || 0;
        document.getElementById('pass-rate').textContent = passRate;
        document.getElementById('progress').style.width = passRate + '%';

        // Populate table
        const tbody = document.getElementById('results');
        (data.tests || []).forEach(test => {
            const parts = test.command.split('/');
            const vendor = parts[0] || 'unknown';
            const cmd = parts.slice(1).join('/') || test.command;

            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td><span class="vendor-badge vendor-${vendor}">${vendor}</span></td>
                <td class="command">${cmd}</td>
                <td class="status-${test.status}">${test.status.toUpperCase()}</td>
                <td class="duration">${(test.duration || 0).toFixed(2)}s</td>
                <td class="error">${test.error || '-'}</td>
            `;
            tbody.appendChild(tr);
        });
    </script>
</body>
</html>
HTMLEOF

# Replace placeholder with actual data
if [[ "$(uname)" == "Darwin" ]]; then
    sed -i '' "s|REPLACE_WITH_DATA|$JSON_ESCAPED|" "$OUTPUT_DIR/report.html"
else
    sed -i "s|REPLACE_WITH_DATA|$JSON_ESCAPED|" "$OUTPUT_DIR/report.html"
fi

echo ""
echo "Reports generated:"
echo "  - JSON: $OUTPUT_DIR/summary.json"
echo "  - HTML: $OUTPUT_DIR/report.html"
echo ""
echo "Open the HTML report with: open $OUTPUT_DIR/report.html"
