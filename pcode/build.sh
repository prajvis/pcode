#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_BIN="$(cd "${ROOT}/../pcode-plugin" && pwd)/bin"
export PATH="${HOME}/.local/go/bin:/usr/local/go/bin:${PATH}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"

mkdir -p "${PLUGIN_BIN}"
go -C "${ROOT}" test ./...
go -C "${ROOT}" build -o "${PLUGIN_BIN}/pcode" ./cmd/pcode
chmod +x "${PLUGIN_BIN}/pcode"
echo "built ${PLUGIN_BIN}/pcode"
