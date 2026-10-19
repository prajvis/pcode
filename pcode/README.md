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
  render/           cursor/codex stream rendering
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
pcode <alias> verify
```

Registry path resolution is documented in the top-level README and in
`pcode --help`.
