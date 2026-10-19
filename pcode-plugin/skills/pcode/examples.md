# pcode examples

## Daily work

```bash
pcode list
pcode web exec 'pwd && git status -sb'
pcode web ai feature "implement the change in this repo; run tests; do not push"
pcode web ai feature "the review comments are: …"
pcode web exec 'git diff && git status -sb'
```

Same chat name continues. A new name starts a new chat.

## Review after the executor

```bash
pcode web exec 'git diff --stat && git diff'
pcode web exec 'git log -5 --oneline'
```

## Register / create

```bash
pcode add --alias web --workspace my-website --repo /home/coder/website
pcode web verify
pcode create --help
```

## Remove alias

Confirm with the user first. Then drop the local alias only (the Coder workspace stays):

```bash
pcode remove web
```
