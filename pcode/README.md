# pcode (Go)

Source for the `pcode` CLI. `build.sh` writes `../pcode-plugin/bin/pcode`
(gitignored).

Needs Go 1.26.5 (see go.mod). Auth is `coder login` (`CODER_URL` /
`CODER_SESSION_TOKEN`, or `~/.config/coderv2/{url,session}`).

Because `go.mod` contains `replace` directives, `go install ...@latest`
is not supported. Build from a clone, use a release tarball, or run
`pcode-plugin/bin/pcode-install`.

## Build and test

```bash
bash build.sh
# or:
go test ./...
go build -o ../pcode-plugin/bin/pcode ./cmd/pcode
```

`go.mod` pins Coder's Tailscale/gVisor forks because
`workspacesdk.DialAgent` needs them and Go does not inherit `replace`
from `github.com/coder/coder/v2`.

## Layout

```text
cmd/pcode/          entrypoint
internal/
  parse/            CLI args
  app/              commands, help text, create playbook
  remote/           Coder HTTP + workspace SSH
  registry/         workspaces.tsv
  threads/          local AI chat metadata
  render/           cursor/codex/pi stream rendering
  execx/            quoting, idle timeout
  config/           env and defaults
```

## Commands

Daily use is in `pcode --help`. Creating a workspace is a separate
playbook: `pcode create --help`.

```text
pcode list
pcode add --alias … --workspace … --repo …
pcode remove <alias>
pcode create --templates
pcode create --show-template <name>
pcode create --alias … --workspace … --template … --parameter 'Name=value'
pcode <alias> exec '<cmd>'
pcode <alias> ai <chat-name> '<prompt>'
pcode <alias> ai --ai-cli pi <chat-name> '<prompt>'   # one-off backend override
pcode <alias> stop <chat-name>
pcode <alias> verify
```

Exit codes: 0 completed, 1 pcode/usage/connection error, 2 agent error,
3 incomplete (no result event), 4 idle timeout, 130 cancelled (signal).
`pcode <alias> stop <chat-name>` stops a remote ai run. Cancelling the
local process (SIGINT/SIGTERM/SIGHUP) also stops the remote agent.

## Pi backend

Register an alias with `--ai-cli pi` to drive the
[Pi coding agent](https://github.com/earendil-works/pi) instead of Cursor:

```bash
pcode add --alias web --workspace my-website --repo /home/coder/website --ai-cli pi
pcode web ai fix-login 'fix the login redirect and run the tests'
```

Each run is `pi --mode json --session-id <uuid> [$CODER_AI_PI_ARGS] -- <prompt>`
inside the repo. pcode generates the session id for a new chat name and
reuses it for follow-ups, so no create-chat step is needed. The JSONL event
stream is rendered like the other backends: tool calls, assistant text, and
a final `pi agent completed` / `pi agent failed`. A provider error
(`stopReason: error`) or an aborted run exits 2; a stream that ends before
`agent_settled` exits 3.

Pi must be installed in the workspace (`curl -fsSL https://pi.dev/install.sh | sh`)
with a provider credential, e.g. a `FIREWORKS_API_KEY` Coder user secret.
Choose the model in the workspace's `~/.pi/agent/settings.json`
(`defaultProvider`, `defaultModel`) or locally with
`CODER_AI_PI_ARGS='--model fireworks/<model-id>'`. `CODER_AI_PI_COMMAND`
overrides the binary (default `pi`). Pi does not ask before tool calls, so
run it only in workspaces you are fine letting it change.

Registry path resolution is documented in the top-level README and in
`pcode --help`.
