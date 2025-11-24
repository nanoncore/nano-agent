#!/bin/bash
set -e

VERSION="${1:-0.1.0}"
DIST_DIR="./dist"
RELEASE_DIR="./release"

echo "=========================================="
echo "  Building nano-agent v${VERSION}"
echo "=========================================="

# Clean previous builds
rm -rf "${DIST_DIR}" "${RELEASE_DIR}"
mkdir -p "${DIST_DIR}" "${RELEASE_DIR}"

# Architectures to build
ARCHS=("amd64" "arm64" "riscv64")

# Cross-compile for each architecture
for ARCH in "${ARCHS[@]}"; do
    echo ""
    echo ">>> Building for linux/${ARCH}..."

    CGO_ENABLED=0 GOOS=linux GOARCH="${ARCH}" go build \
        -ldflags="-s -w -X main.Version=${VERSION}" \
        -o "${DIST_DIR}/nano-agent-linux-${ARCH}" \
        ./cmd/nano-agent

    echo "    Built: ${DIST_DIR}/nano-agent-linux-${ARCH}"
done

# Install nfpm if not available
if ! command -v nfpm &> /dev/null && ! command -v ~/go/bin/nfpm &> /dev/null; then
    echo ""
    echo ">>> Installing nfpm..."
    go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
fi

# Use nfpm from go bin if not in PATH
NFPM_CMD="nfpm"
if ! command -v nfpm &> /dev/null; then
    NFPM_CMD="$HOME/go/bin/nfpm"
fi

# Update version in nfpm configs and build .deb packages
for ARCH in "${ARCHS[@]}"; do
    echo ""
    echo ">>> Creating .deb package for ${ARCH}..."

    # Create a temporary nfpm config with correct version
    NFPM_CONFIG="./packaging/nfpm-${ARCH}.yaml"
    NFPM_TMP="/tmp/nfpm-${ARCH}-${VERSION}.yaml"

    sed "s/^version:.*/version: \"${VERSION}\"/" "${NFPM_CONFIG}" > "${NFPM_TMP}"

    ${NFPM_CMD} package \
        --config "${NFPM_TMP}" \
        --packager deb \
        --target "${RELEASE_DIR}/"

    rm -f "${NFPM_TMP}"
    echo "    Created: ${RELEASE_DIR}/nano-agent_${VERSION}_${ARCH}.deb"
done

# Copy binaries to release directory
for ARCH in "${ARCHS[@]}"; do
    cp "${DIST_DIR}/nano-agent-linux-${ARCH}" "${RELEASE_DIR}/"
done

# Generate checksums
echo ""
echo ">>> Generating checksums..."
cd "${RELEASE_DIR}"
sha256sum * > checksums.txt
cd - > /dev/null

echo ""
echo "=========================================="
echo "  Release artifacts ready in ${RELEASE_DIR}/"
echo "=========================================="
ls -la "${RELEASE_DIR}/"

echo ""
echo "To create a GitHub release:"
echo ""
echo "  gh release create v${VERSION} ${RELEASE_DIR}/* \\"
echo "    --repo nanoncore/nano-agent \\"
echo "    --title \"nano-agent v${VERSION}\" \\"
echo "    --notes \"Release notes here\""
echo ""
