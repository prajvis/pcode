---
name: pcode
description: >-
  Do coding, git, tests, and AI work inside the user's Coder remote workspace.
  Use eagerly when they mention Coder, a remote workspace, a workspace alias,
  a hosted repo, or want implementation on the workspace.
  You are the manager/reviewer. Delegate non-trivial implementation
  to `pcode <alias> ai` and use `pcode <alias> exec` for shell, git, and review.
  Start with `pcode list`. Reuse the same chat name to continue; a new
  name starts a new chat.
  If several aliases could match, ask. If none exists, register one after confirming.
---

# pcode

The repo lives in a Coder workspace. That workspace is the execution
environment. Do not implement in this local sandbox, even if a checkout exists
on this machine.

1. `pcode list` — pick the alias. Ask if more than one could fit.
2. Shell: `pcode web exec 'pwd && git status -sb'`
3. AI: `pcode web ai review "look at the latest diff"`
4. Same chat name continues that conversation. A new name starts a new one. Prefer continuing when working on the same topic. Create a separate one for code reviews.

## Recommended workflow

You are the manager. The workspace AI is the executor.

For non-trivial changes, do not write the code yourself. Brief the workspace
AI and review what it did.

1. You act as manager. The Coder workspace agent acts as executor.
2. Give one precise task: repo areas, expected design, constraints, tests to
   run, and whether to commit or push.
3. Send it with `pcode <alias> ai <chat-name> "<task>"`.
4. Wait for that run to finish. Continue the same chat name for follow-ups.
5. Review before reporting. Prefer a **new chat name** so review stays off
   the implementation thread:
   `pcode <alias> ai review "<review-prompt>"`
   Write `<review-prompt>` for this change. It should tell the reviewer to
   inspect the diff, name the files that matter, check tests/build, and
   say whether the original constraints were met. Do not reuse a canned
   one-liner.
   Or skip the review chat and inspect it yourself with `pcode <alias> exec`
   (`git diff`, `git status`, tests).
6. Only then tell the user it is done.

Do not have two executors modifying the same repo at once. While a workspace
agent is implementing, do not run build/test/git writes or edit code in
parallel unless you need read-only monitoring. Skip extra agents for simple
checks — `exec` is enough.

Simple read-only questions (`pwd`, `git status`, `rg`) can be `exec` without
an AI delegate.

If you are unsure about a flag, run `pcode --help`. Do not call help on every
turn. Do not invent wrappers. Do not `coder ssh`. If `pcode` fails, stop and
report it.

## Register or create

Existing workspace:

```bash
pcode add --alias web --workspace my-website --repo /home/coder/website
pcode web verify
```

New workspace: only if they asked. Run `pcode create --help` and follow it
with the user.

## Remove alias

Do not remove an alias on your own. Confirm with the user first, including
which alias. `remove` (also `rm`) drops the local alias only; it does not
delete the Coder workspace.

```bash
pcode remove web
```

If `pcode` is missing, source `/shared/.bashrc` and retry. If it is still
missing:

```bash
bash <plugin-root>/scripts/install-path.sh --target-root /shared
source /shared/.bashrc
```

If `<plugin-root>/bin/pcode` itself does not exist, install it and retry:
`<plugin-root>/bin/pcode-install` (or `--dir` to a writable directory on PATH).

Auth is whatever `coder login` already set. AI keys live as Coder user secrets
inside the workspace.
