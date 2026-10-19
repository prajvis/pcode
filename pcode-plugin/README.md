# pcode plugin

pcode is a CLI for AI agents: it runs shell commands and AI coding tools
inside Coder workspaces. The skill in `skills/pcode/` tells the agent how
to use it.

Install this folder as a plugin in Prajvis, Cursor, Claude Code, or Codex, then
run `bin/pcode-install` once. That writes the release binary to
`bin/pcode`, which is the command on PATH. `bin/pcode` is not stored in
git.

```bash
./bin/pcode-install
```

If this directory is not writable, pass `--dir` (or set
`PCODE_INSTALL_DIR`) to a writable directory on PATH. A local build
from `../pcode/build.sh` writes `bin/pcode` as well.

Claude Code puts `bin/` on PATH while the plugin is enabled. Human
shells and some other hosts may need PATH set themselves.

`scripts/install-path.sh --target-root <dir>` is optional. It appends a
PATH snippet to `<dir>/.bashrc` (for example `/shared`).

Alias registry location is resolved by the CLI; see the top-level
README.
