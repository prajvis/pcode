#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_BIN="$(cd "${ROOT}/../pcode-plugin" && pwd)/bin"
export PATH="${HOME}/.local/go/bin:/usr/local/go/bin:${PATH}"
# Default to the go.mod version: newer Go (1.27+) fails to link the tailscale fork Coder needs.
export GOTOOLCHAIN="${GOTOOLCHAIN:-go$(awk '/^go /{print $2}' "${ROOT}/go.mod")}"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"

mkdir -p "${PLUGIN_BIN}"
go -C "${ROOT}" test ./...
go -C "${ROOT}" build -o "${PLUGIN_BIN}/pcode" ./cmd/pcode
chmod +x "${PLUGIN_BIN}/pcode"
echo "built ${PLUGIN_BIN}/pcode"
