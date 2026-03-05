#!/bin/bash

# UAT Test for Combined Archive Feature in home-ci-reporter
# This test validates the complete workflow:
# 1. Create test files
# 2. Create a tar.gz archive
# 3. Encode to base64
# 4. Create payload JSON with archive metadata
# 5. Test extraction with home-ci-reporter
# 6. Validate extracted files

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMP_DIR=$(mktemp -d)
REPORTER_BIN="${SCRIPT_DIR}/../../home-ci-reporter"

echo "🧪 UAT Test: Combined Archive Feature"
echo "======================================"
echo "Test directory: ${TEMP_DIR}"
echo "Reporter binary: ${REPORTER_BIN}"

# Cleanup function
cleanup() {
    echo "🧹 Cleaning up temporary files..."
    rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT

# Step 1: Create test files
echo ""
echo "📝 Step 1: Creating test files"
mkdir -p "${TEMP_DIR}/input"

# Create test log file
cat > "${TEMP_DIR}/input/test.log" << EOF
=== Test Execution Log ===
Start time: $(date -u +%Y-%m-%dT%H:%M:%SZ)
Branch: feature/combined-archive-test
Commit: abc123def456789

Phase: Build
Status: passed
Message: Build completed successfully

Phase: Test
Status: failed
Message: Unit tests failed with 2 errors

Phase: Cleanup
Status: passed
Message: Cleanup completed successfully

=== Test Complete ===
Duration: 45.239 seconds
Overall Status: FAILED
EOF

# Create test e2e-report.yaml
cat > "${TEMP_DIR}/input/e2e-report.yaml" << 'EOF'
# E2E Test Report
test_run:
  start_time: 2026-03-05T09:30:00.123456789Z
  runner: uat-test-runner
  project_name: home-ci-combined-archive
environment:
  os: linux
  arch: amd64
  shell: home-ci-reporter
steps:
  - phase: build
    status: passed
    message: Build completed successfully
    timestamp: 2026-03-05T09:30:15.456789Z
  - phase: test
    status: failed
    message: Unit tests failed
    timestamp: 2026-03-05T09:31:00.789123Z
  - phase: cleanup
    status: passed
    message: Cleanup completed successfully
    timestamp: 2026-03-05T09:31:30.456789Z
summary:
  end_time: 2026-03-05T09:31:45.123456789Z
  duration_seconds: 105
  total_steps: 3
  passed_steps: 2
  failed_steps: 1
  overall_status: failed
  success_rate: 67%
EOF

# Create test result.json
cat > "${TEMP_DIR}/input/result.json" << EOF
{
  "branch": "feature/combined-archive-test",
  "commit": "abc123def456789",
  "log_file": "test.log",
  "start_time": "$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)",
  "end_time": "$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)",
  "duration": 105000000000,
  "success": false,
  "timed_out": false,
  "cleanup_executed": true,
  "cleanup_success": true,
  "github_actions_notified": true,
  "github_actions_success": true,
  "error_message": "unit tests failed"
}
EOF

# Step 2: Create tar.gz archive
echo ""
echo "📦 Step 2: Creating tar.gz archive"
cd "${TEMP_DIR}/input"
tar -czf "${TEMP_DIR}/combined-archive.tar.gz" test.log e2e-report.yaml result.json
cd - > /dev/null

# Step 3: Encode to base64
echo ""
echo "🔐 Step 3: Encoding archive to base64"
ARCHIVE_B64=$(base64 -w 0 "${TEMP_DIR}/combined-archive.tar.gz")
ARCHIVE_SIZE=$(stat -c%s "${TEMP_DIR}/combined-archive.tar.gz")
LOG_SIZE=$(stat -c%s "${TEMP_DIR}/input/test.log")
YAML_SIZE=$(stat -c%s "${TEMP_DIR}/input/e2e-report.yaml")
JSON_SIZE=$(stat -c%s "${TEMP_DIR}/input/result.json")
TOTAL_ORIGINAL_SIZE=$((LOG_SIZE + YAML_SIZE + JSON_SIZE))

echo "  Archive size: ${ARCHIVE_SIZE} bytes"
echo "  Original total: ${TOTAL_ORIGINAL_SIZE} bytes"
echo "  Compression ratio: $(echo "scale=1; 100 * ${ARCHIVE_SIZE} / ${TOTAL_ORIGINAL_SIZE}" | bc)%"

# Step 4: Create payload JSON with real archive
echo ""
echo "📋 Step 4: Creating payload JSON with real archive data"
cat > "${TEMP_DIR}/test-payload.json" << EOF
{
  "success": false,
  "source": "home-ci",
  "branch": "feature/combined-archive-test",
  "commit": "abc123def456789",
  "artifact_name": "log-feature_combined-archive-test-abc123de",
  "artifacts": {
    "combined-archive.tar.gz": {
      "content": "${ARCHIVE_B64}",
      "type": "archive",
      "compressed": true,
      "truncated": false,
      "original_size": ${TOTAL_ORIGINAL_SIZE},
      "files": [
        {
          "name": "test.log",
          "type": "log",
          "truncated": false,
          "original_size": ${LOG_SIZE}
        },
        {
          "name": "e2e-report.yaml",
          "type": "e2e-report",
          "truncated": false,
          "original_size": ${YAML_SIZE}
        },
        {
          "name": "result.json",
          "type": "result",
          "truncated": false,
          "original_size": ${JSON_SIZE}
        }
      ]
    },
    "metadata": {
      "content": "",
      "type": "metadata",
      "compressed": false,
      "truncated": false,
      "original_size": 0
    }
  },
  "metadata": {
    "branch": "feature/combined-archive-test",
    "commit": "abc123def456789",
    "success": false
  },
  "timestamp": "$(date +%s)"
}
EOF

# Step 5: Test extraction with home-ci-reporter
echo ""
echo "🔍 Step 5: Testing extraction with home-ci-reporter"

# Check if binary exists
if [[ ! -f "${REPORTER_BIN}" ]]; then
    echo "❌ ERROR: home-ci-reporter binary not found at ${REPORTER_BIN}"
    echo "   Please build the binary first with: go build"
    exit 1
fi

# Make binary executable
chmod +x "${REPORTER_BIN}"

# Create output directory
mkdir -p "${TEMP_DIR}/extracted"

# Run extraction
echo "  Running: ${REPORTER_BIN} extract ${TEMP_DIR}/test-payload.json ${TEMP_DIR}/extracted"
"${REPORTER_BIN}" extract "${TEMP_DIR}/test-payload.json" "${TEMP_DIR}/extracted"

# Step 6: Validate extracted files
echo ""
echo "✅ Step 6: Validating extracted files"

# Check extracted files exist
EXPECTED_FILES=("test.log" "e2e-report.yaml" "result.json")
ALL_FOUND=true

for file in "${EXPECTED_FILES[@]}"; do
    if [[ -f "${TEMP_DIR}/extracted/${file}" ]]; then
        echo "  ✅ Found: ${file}"

        # Compare file contents
        if diff -q "${TEMP_DIR}/input/${file}" "${TEMP_DIR}/extracted/${file}" > /dev/null; then
            echo "     ✅ Content matches original"
        else
            echo "     ❌ Content differs from original"
            ALL_FOUND=false
        fi

        # Show file sizes
        ORIGINAL_SIZE=$(stat -c%s "${TEMP_DIR}/input/${file}")
        EXTRACTED_SIZE=$(stat -c%s "${TEMP_DIR}/extracted/${file}")
        echo "     📏 Size: ${EXTRACTED_SIZE} bytes (original: ${ORIGINAL_SIZE} bytes)"

    else
        echo "  ❌ Missing: ${file}"
        ALL_FOUND=false
    fi
done

# Step 7: Test summary generation
echo ""
echo "📊 Step 7: Testing summary generation"
echo "  Running: ${REPORTER_BIN} summary ${TEMP_DIR}/test-payload.json"
"${REPORTER_BIN}" summary "${TEMP_DIR}/test-payload.json"

# Final result
echo ""
echo "🏁 UAT Test Results"
echo "=================="

if [[ "${ALL_FOUND}" == "true" ]]; then
    echo "✅ SUCCESS: All tests passed!"
    echo "   - Combined archive created successfully"
    echo "   - Archive extraction working correctly"
    echo "   - All files extracted with correct content"
    echo "   - Summary generation working"
    echo ""
    echo "🚀 Combined archive feature is ready for production!"
    exit 0
else
    echo "❌ FAILURE: Some tests failed!"
    echo "   Please check the errors above and fix the implementation."
    exit 1
fi