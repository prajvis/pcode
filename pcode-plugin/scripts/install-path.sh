#!/usr/bin/env bash
set -euo pipefail

# Optional helper: put the plugin bin/ on PATH from a bashrc under
# --target-root (for example /shared). Does not copy pcode.
# Data (workspaces.tsv) stays under <target-root>/.prajvis-coder/data
# so existing shared-root installs keep working.

PLUGIN_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET_ROOT="${CODER_RUNTIME_TARGET_ROOT:-/shared}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target-root)
      TARGET_ROOT="$2"
      shift 2
      ;;
    -h|--help)
      cat <<EOF
Usage:
  install-path.sh [--target-root <dir>]

Optional. Adds ${PLUGIN_ROOT}/bin to PATH from:
  <target-root>/.bashrc

Creates (if missing):
  <target-root>/.prajvis-coder/data/workspaces.tsv

Does not install the binary. Get bin/pcode with either:
  ${PLUGIN_ROOT}/bin/pcode-install
  bash ${PLUGIN_ROOT}/../pcode/build.sh
EOF
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 2
      ;;
  esac
done

if [[ ! -x "${PLUGIN_ROOT}/bin/pcode" ]]; then
  echo "note: ${PLUGIN_ROOT}/bin/pcode is not installed yet; run ${PLUGIN_ROOT}/bin/pcode-install" >&2
fi

DATA_HOME="${TARGET_ROOT}/.prajvis-coder"
DATA_DIR="${DATA_HOME}/data"
BASHRC_PATH="${TARGET_ROOT}/.bashrc"
WORKSPACES_PATH="${DATA_DIR}/workspaces.tsv"

mkdir -p "$DATA_DIR"
if [[ ! -f "$WORKSPACES_PATH" ]]; then
  cat >"$WORKSPACES_PATH" <<'EOF'
# alias	workspace	repo	ai_cli
EOF
fi

if [[ ! -f "$BASHRC_PATH" ]]; then
  : > "$BASHRC_PATH"
fi

python3 - "$BASHRC_PATH" "$PLUGIN_ROOT" "$DATA_HOME" <<'PY'
from pathlib import Path
import sys

bashrc = Path(sys.argv[1])
plugin_root = sys.argv[2]
data_home = sys.argv[3]
start = "# >>> pcode plugin >>>"
end = "# <<< pcode plugin <<<"
block = f"""{start}
export PCODE_PLUGIN_HOME="{plugin_root}"
export CODER_RUNTIME_HOME="${{CODER_RUNTIME_HOME:-{data_home}}}"
export CODER_RUNTIME_DATA_DIR="$CODER_RUNTIME_HOME/data"
export CODER_WORKSPACE_REFERENCE_FILE="$CODER_RUNTIME_DATA_DIR/workspaces.tsv"
export CODER_WORKSPACE_REGISTRY_FILE="$CODER_WORKSPACE_REFERENCE_FILE"
export CODER_DEFAULT_AI_CLI="${{CODER_DEFAULT_AI_CLI:-cursor}}"
case ":$PATH:" in
  *":$PCODE_PLUGIN_HOME/bin:"*) ;;
  *) export PATH="$PCODE_PLUGIN_HOME/bin:$PATH" ;;
esac
{end}
"""
text = bashrc.read_text() if bashrc.exists() else ""
if start in text and end in text:
    before, rest = text.split(start, 1)
    _, after = rest.split(end, 1)
    text = before.rstrip("\n") + "\n" + block + after.lstrip("\n")
elif "# >>> Prajvis Coder runtime >>>" in text and "# <<< Prajvis Coder runtime <<<" in text:
    before, rest = text.split("# >>> Prajvis Coder runtime >>>", 1)
    _, after = rest.split("# <<< Prajvis Coder runtime <<<", 1)
    text = before.rstrip("\n") + "\n" + block + after.lstrip("\n")
else:
    if text and not text.endswith("\n"):
        text += "\n"
    text += block
bashrc.write_text(text)
PY

echo "Plugin bin: ${PCODE_SHIM}"
echo "Registry:   ${WORKSPACES_PATH}"
echo "bashrc:     ${BASHRC_PATH}"
echo
echo "Load it with:"
echo "  source \"${BASHRC_PATH}\""
echo
echo "Then:"
echo "  pcode --help"
echo "  pcode list"
