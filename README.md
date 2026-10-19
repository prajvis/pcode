# pcode

pcode is a CLI for AI agents. It lets an agent run shell commands and AI
coding tools inside Coder workspaces. A manager agent can brief coding
agents in parallel worktrees, then review their diffs, without doing the
implementation in its own session.

It is meant to be called by agents, not typed by people. The plugin ships
a skill that tells the agent when and how to use it. You can call it from
[Prajvis](https://prajvis.com) agents, where we use it for our own
engineering work, and from Claude Code, Cursor and Codex.

It talks to a Coder deployment you already use. Authentication is
`coder login`; pcode reuses that session.

## Requirements

- A [Coder](https://coder.com/) deployment (self-hosted or Coder Cloud)
- The [Coder CLI](https://github.com/coder/coder) installed and `coder login` completed against your URL

## Install

### Plugin (Prajvis, Claude Code, Cursor, Codex)

Install the `pcode-plugin/` directory as a plugin, then run
`pcode-plugin/bin/pcode-install` once. That writes the release binary
to `pcode-plugin/bin/pcode`, which is the command on PATH (linux and
darwin, amd64 and arm64). `bin/pcode` is not stored in git.

If the plugin directory is not writable, pass `--dir` (or set
`PCODE_INSTALL_DIR`) to a writable directory that is on PATH:

```bash
pcode-plugin/bin/pcode-install --dir ~/.local/bin
```

A local build writes the same path:

```bash
bash pcode/build.sh
```

### Shell

Download a release tarball from
https://github.com/prajvis/pcode/releases, extract it, and put `pcode`
on your `PATH`. Or clone this repository and build (see below).

Because `go.mod` contains `replace` directives for Coder's Tailscale and
gVisor forks, `go install ...@latest` is not supported.

## Quickstart

```bash
pcode add --alias web --workspace my-website --repo /home/coder/website
pcode list
pcode web exec 'pwd && git status -sb'
pcode web ai feature "implement the change; run tests; do not push"
pcode web verify
pcode remove web
```

`create` provisions a Coder workspace from a template; see
`pcode create --help`. Confirm with the user before `remove`. `remove`
drops the local alias only; it does not delete the Coder workspace.

Same alias and chat name resume a thread. A new name starts a new chat;
use a separate name for review.

## Registry

Aliases live in a TSV file, not in this repository. The path is resolved
in this order:

1. `$CODER_WORKSPACE_REFERENCE_FILE` if set
2. `$CODER_RUNTIME_HOME/data/workspaces.tsv` if `CODER_RUNTIME_HOME` is set
3. `$PCODE_HOME/data/workspaces.tsv` if `PCODE_HOME` is set
4. `/shared/.prajvis-coder/data/workspaces.tsv` if that directory exists
5. `${XDG_CONFIG_HOME:-$HOME/.config}/pcode/data/workspaces.tsv`

```text
# alias<TAB>workspace<TAB>repo<TAB>ai_cli
web	my-website	/home/coder/website	cursor
home	alice/home-lab	/home/coder/home	cursor
```

## Building from source

Go 1.26.5 (see pcode/go.mod).

```bash
cd pcode
bash build.sh
# writes pcode-plugin/bin/pcode
```

Or:

```bash
cd pcode
go test ./...
go build -o ../pcode-plugin/bin/pcode ./cmd/pcode
```

`go.mod` pins Coder's Tailscale and gVisor forks because
`workspacesdk.DialAgent` needs them and Go does not inherit `replace`
from `github.com/coder/coder/v2`.

`pcode-plugin/scripts/install-path.sh` is optional. It appends a PATH
snippet to a bashrc under `--target-root` (for example `/shared`) so
human shells can find `pcode-plugin/bin`. Claude Code puts `bin/` on
PATH while the plugin is enabled; Cursor and Codex may still need that
snippet.

## Layout

```text
pcode/            CLI source
pcode-plugin/     Claude Code, Cursor, and Codex plugin and skill
  bin/pcode         CLI binary (not in git; run bin/pcode-install)
  bin/pcode-install downloads the matching release into bin/pcode
```

## License

Apache License 2.0. Prebuilt binaries link `github.com/coder/coder/v2`
(AGPL-3.0); see NOTICE and THIRD_PARTY_LICENSES/. Trademark notes are in
TRADEMARKS.md.
