// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prajvis/pcode/pcode/internal/config"
	"github.com/prajvis/pcode/pcode/internal/execx"
	"github.com/prajvis/pcode/pcode/internal/parse"
	"github.com/prajvis/pcode/pcode/internal/registry"
	"github.com/prajvis/pcode/pcode/internal/remote"
	"github.com/prajvis/pcode/pcode/internal/render"
	"github.com/prajvis/pcode/pcode/internal/threads"
)

type App struct {
	Cfg     config.Config
	Reg     *registry.Registry
	Threads *threads.Store
	Client  remote.Client
	Stdout  io.Writer
	Stderr  io.Writer
	Version string
	Dial    func(ctx context.Context, workspace string) (remote.Session, error)
}

func New(cfg config.Config) *App {
	return &App{
		Cfg:     cfg,
		Reg:     registry.Open(cfg.RegistryFile),
		Threads: &threads.Store{Root: cfg.AIStateDir},
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
}

func (a *App) Run(ctx context.Context, args []string) error {
	req, err := parse.Parse(args)
	if err != nil {
		return err
	}
	switch req.Kind {
	case parse.KindHelp:
		fmt.Fprint(a.out(), Usage())
		return nil
	case parse.KindVersion:
		v := a.Version
		if v == "" {
			v = "dev"
		}
		fmt.Fprintln(a.out(), "pcode "+v)
		return nil
	case parse.KindList:
		return a.list()
	case parse.KindAdd:
		return a.add(req)
	case parse.KindRemove:
		return a.remove(req)
	case parse.KindCreateHelp:
		fmt.Fprint(a.out(), CreateUsage())
		return nil
	case parse.KindCreateTemplates:
		return a.createTemplates(ctx)
	case parse.KindCreateShow:
		return a.createShow(ctx, req)
	case parse.KindCreate:
		return a.create(ctx, req)
	case parse.KindExec:
		return a.exec(ctx, req)
	case parse.KindAI:
		return a.ai(ctx, req)
	case parse.KindShell:
		return a.shell(ctx, req)
	case parse.KindThreads:
		return a.listThreads(req)
	case parse.KindLogs:
		return a.logs(ctx, req)
	case parse.KindVerify:
		return a.verify(ctx, req)
	case parse.KindStop:
		return a.stop(ctx, req)
	default:
		return fmt.Errorf("unhandled command")
	}
}

func Usage() string {
	return `pcode — run shell commands and AI inside a registered Coder workspace.

Work happens in the remote workspace, not the local sandbox. Look up the
alias in the TSV registry, then exec or ai. Do not use c<alias>x, c<alias>ai,
or raw coder ssh.

USAGE
  pcode list
  pcode add --alias <alias> --workspace <workspace> --repo <repo> [--ai-cli cursor]
  pcode remove <alias>
  pcode create --help
  pcode version
  pcode <alias> exec <command>
  pcode <alias> ai <chat-title> <prompt>
  pcode <alias> threads
  pcode <alias> verify
  pcode <alias> logs
  pcode <alias> shell
  pcode <alias> stop <chat-title>

COMMANDS
  list      Print registered aliases (alias, Coder workspace, repo, ai cli).
  add       Register or update an alias. Does not create the Coder workspace.
            --alias       short name, ^[a-z][a-z0-9_-]*$
            --workspace   Coder name or owner/name (e.g. my-app or owner/my-app)
            --repo        absolute repo path inside the workspace
            --ai-cli      cursor (default), codex, or claude
  remove    Drop a local alias. Does not delete the Coder workspace.
            Confirm with the user before running this. Also: pcode rm <alias>
  create    Create a Coder workspace from a template, then register the alias.
            Templates have many fields. Run pcode create --help and work with
            the user. Do not use this unless they asked to create a workspace.
  version   Print the pcode version. Also: pcode --version
  exec      Run a command after cd'ing into the alias repo. Also: pcode <alias> x
  ai        Send a prompt to the alias AI CLI (cursor agent by default).
            Same alias + same chat-title resumes the stored thread.
            A different title starts a new chat. There is no follow command.
  threads   List stored chat titles and thread ids for the alias.
  verify    Start the workspace if needed, then pwd + git status in the repo.
  logs      Print the latest Coder build logs for the workspace.
  shell     Interactive SSH session into the workspace.
  stop      Stop a running ai job for this alias and chat title.
            Sends SIGTERM to the remote process group, then SIGKILL.
            If nothing is running, says so and exits 0.
            Cancelling the local pcode process also stops the remote agent.

EXIT CODES
  0   completed
  1   pcode/usage/connection error
  2   agent reported an error
  3   incomplete (stream ended without a result)
  4   idle timeout
  130 cancelled (signal)

REGISTRY
  TSV file, resolved in this order:
    $CODER_WORKSPACE_REFERENCE_FILE
    $CODER_RUNTIME_HOME/data/workspaces.tsv
    $PCODE_HOME/data/workspaces.tsv
    /shared/.prajvis-coder/data/workspaces.tsv  (if that directory exists)
    ${XDG_CONFIG_HOME:-$HOME/.config}/pcode/data/workspaces.tsv
  Format:
    # alias<TAB>workspace<TAB>repo<TAB>ai_cli
    web	my-website	/home/coder/website	cursor

AUTH
  Reuses coder login: CODER_URL and CODER_SESSION_TOKEN, or
  ~/.config/coderv2/url and ~/.config/coderv2/session.

EXAMPLES
  pcode list
  pcode web exec 'pwd && git status -sb'
  pcode web ai review "summarize the latest diff"
  pcode web ai review "now fix the comments you just found"
  pcode web stop review
`
}

func (a *App) list() error {
	if err := a.Reg.Ensure(); err != nil {
		return err
	}
	records, err := a.Reg.List()
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Fprintln(a.out(), "No registered workspaces.")
		return nil
	}
	fmt.Fprintf(a.out(), "%-16s %-28s %-40s %s\n", "ALIAS", "WORKSPACE", "REPO", "AI")
	for _, rec := range records {
		fmt.Fprintf(a.out(), "%-16s %-28s %-40s %s\n", rec.Alias, rec.Workspace, rec.Repo, rec.AICLI)
	}
	return nil
}

func (a *App) add(req parse.Request) error {
	rec := registry.Record{
		Alias:     req.AddAlias,
		Workspace: req.Workspace,
		Repo:      req.Repo,
		AICLI:     req.AICLI,
	}
	if err := a.Reg.Upsert(rec); err != nil {
		return err
	}
	fmt.Fprintf(a.out(), "registered %s -> %s %s (%s)\n", rec.Alias, rec.Workspace, rec.Repo, rec.AICLI)
	return nil
}

func (a *App) remove(req parse.Request) error {
	if err := a.Reg.Remove(req.AddAlias); err != nil {
		return err
	}
	fmt.Fprintf(a.out(), "removed alias %s\n", req.AddAlias)
	return nil
}

func (a *App) exec(ctx context.Context, req parse.Request) error {
	rec, err := a.Reg.Get(req.Alias)
	if err != nil {
		return err
	}
	sess, err := a.dial(ctx, rec.Workspace)
	if err != nil {
		return err
	}
	defer sess.Close()
	cmd := "cd " + execx.BashQuote(rec.Repo) + " && " + req.Command
	return sess.Run(ctx, cmd, a.out(), a.err())
}

func (a *App) shell(ctx context.Context, req parse.Request) error {
	rec, err := a.Reg.Get(req.Alias)
	if err != nil {
		return err
	}
	sess, err := a.dial(ctx, rec.Workspace)
	if err != nil {
		return err
	}
	defer sess.Close()
	return sess.Shell(ctx)
}

func (a *App) listThreads(req parse.Request) error {
	if _, err := a.Reg.Get(req.Alias); err != nil {
		return err
	}
	rows, err := a.Threads.List(req.Alias)
	if err != nil {
		return err
	}
	fmt.Fprint(a.out(), threads.FormatList(rows))
	return nil
}

func (a *App) logs(ctx context.Context, req parse.Request) error {
	rec, err := a.Reg.Get(req.Alias)
	if err != nil {
		return err
	}
	client, err := a.client()
	if err != nil {
		return err
	}
	return client.Logs(ctx, rec.Workspace)
}

func (a *App) verify(ctx context.Context, req parse.Request) error {
	rec, err := a.Reg.Get(req.Alias)
	if err != nil {
		return err
	}
	sess, err := a.dial(ctx, rec.Workspace)
	if err != nil {
		return err
	}
	defer sess.Close()
	cmd := "cd " + execx.BashQuote(rec.Repo) + " && pwd && git rev-parse --show-toplevel && git status -sb"
	if err := sess.Run(ctx, cmd, a.out(), a.err()); err != nil {
		return err
	}
	fmt.Fprintf(a.err(), "pcode: %s ok (%s -> %s)\n", rec.Alias, rec.Workspace, rec.Repo)
	return nil
}

func (a *App) ai(ctx context.Context, req parse.Request) error {
	rec, err := a.Reg.Get(req.Alias)
	if err != nil {
		return err
	}
	sess, err := a.dial(ctx, rec.Workspace)
	if err != nil {
		return err
	}
	defer sess.Close()

	cli := config.NormalizeAICLI(rec.AICLI)
	title := strings.TrimSpace(req.Title)
	resume := ""
	if meta, err := a.Threads.Read(rec.Alias, title); err == nil && strings.TrimSpace(meta.ThreadID) != "" {
		resume = meta.ThreadID
		fmt.Fprintf(a.err(), "resuming chat %s (%s)\n", title, resume)
	}

	if err := a.refuseIfRunning(ctx, sess, rec.Alias, title); err != nil {
		return err
	}

	switch cli {
	case "cursor":
		if resume == "" {
			fmt.Fprintf(a.err(), "creating cursor chat %s…\n", title)
			out, err := sess.Output(ctx, "cd "+execx.BashQuote(rec.Repo)+" && unset CURSOR_AGENT_OUTPUT_FORMAT CURSOR_OUTPUT_FORMAT && "+a.Cfg.CursorCommand+" create-chat")
			if err != nil {
				return fmt.Errorf("create cursor chat: %w", err)
			}
			id, ok := ExtractUUID(out)
			if !ok {
				return fmt.Errorf("create cursor chat: no chat id in output: %s", strings.TrimSpace(out))
			}
			resume = id
			if err := a.Threads.Write(rec.Alias, title, cli, rec.Workspace, rec.Repo, resume); err != nil {
				return err
			}
		}
		fmt.Fprintf(a.err(), "sending prompt…\n")
		if err := a.runCursor(ctx, sess, rec, req.Prompt, resume, title); err != nil {
			return err
		}
		return a.Threads.Write(rec.Alias, title, cli, rec.Workspace, rec.Repo, resume)
	case "codex":
		threadID, err := a.runCodex(ctx, sess, rec, req.Prompt, resume, title)
		if err != nil {
			return err
		}
		if threadID == "" {
			threadID = resume
		}
		if threadID == "" {
			return fmt.Errorf("codex did not return a thread id")
		}
		return a.Threads.Write(rec.Alias, title, cli, rec.Workspace, rec.Repo, threadID)
	case "claude":
		cmd := "cd " + execx.BashQuote(rec.Repo) + " && " + a.Cfg.ClaudeCommand + " " + execx.BashQuote(req.Prompt)
		return a.runPlain(ctx, sess, rec.Alias, title, cmd)
	default:
		return fmt.Errorf("unsupported ai cli %q (cursor, codex, claude)", cli)
	}
}

func (a *App) runCursor(ctx context.Context, sess remote.Session, rec registry.Record, prompt, resume, title string) error {
	flags := "-p"
	if a.Cfg.CursorStream {
		flags += " --output-format stream-json --stream-partial-output"
	} else {
		flags += " --output-format text"
	}
	if a.Cfg.CursorForce {
		flags += " --force"
	}
	inner := "cd " + execx.BashQuote(rec.Repo) + " && unset CURSOR_AGENT_OUTPUT_FORMAT CURSOR_OUTPUT_FORMAT && " + a.Cfg.CursorCommand + " " + flags
	if resume != "" {
		inner += " --resume " + execx.BashQuote(resume)
	}
	inner += " " + execx.BashQuote(prompt)
	cmd := execx.WrapScript(rec.Alias, title, inner)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := sess.RunWithStdin(runCtx, cmd, stdinR, pw, a.err())
		_ = pw.CloseWithError(err)
		errCh <- err
	}()

	dec := &render.CursorDecoder{}
	var rendered bytes.Buffer
	src := io.Reader(pr)
	copyErrCh := make(chan error, 1)
	if a.Cfg.IdleTimeout > 0 {
		ipr, ipw := io.Pipe()
		go func() {
			err := execx.CopyIdleBusy(runCtx, pr, ipw, a.Cfg.IdleTimeout, dec.Busy)
			_ = ipw.CloseWithError(err)
			copyErrCh <- err
		}()
		src = ipr
	} else {
		copyErrCh <- nil
	}
	resultCh := make(chan struct{})
	var resultOnce sync.Once
	dec.OnResult = func() { resultOnce.Do(func() { close(resultCh) }) }
	stopGrace := a.startStreamGrace(resultCh, cancel, stdinW, pr, pw)
	res := dec.Render(src, io.MultiWriter(a.out(), &rendered))
	stopGrace()
	copyErr := <-copyErrCh

	idle := errors.Is(copyErr, execx.ErrIdleTimeout)
	parentCancel := ctx.Err() != nil
	if idle || parentCancel || res.Err != nil || !res.Completed {
		cancel()
		_ = stdinW.Close()
		_ = pr.Close()
		_ = pw.Close()
		a.killRemote(sess, rec.Alias, title)
	}
	runErr := waitErr(errCh, 8*time.Second)
	if runErr != nil && strings.Contains(runErr.Error(), "timed out waiting") {
		cancel()
		_ = stdinW.Close()
		_ = pr.Close()
		_ = pw.Close()
		a.killRemote(sess, rec.Alias, title)
	}
	if idle {
		return fmt.Errorf("%w after %s", execx.ErrIdleTimeout, a.Cfg.IdleTimeout)
	}
	if parentCancel {
		return ctx.Err()
	}
	if res.Err != nil {
		return res.Err
	}
	if res.Completed {
		return nil
	}
	if runErr != nil {
		return runErr
	}
	return fmt.Errorf("%w: no result event", render.ErrIncomplete)
}

func (a *App) runCodex(ctx context.Context, sess remote.Session, rec registry.Record, prompt, resume, title string) (string, error) {
	flags := "--json"
	if a.Cfg.CodexForce {
		flags += " --dangerously-bypass-approvals-and-sandbox"
	}
	inner := "cd " + execx.BashQuote(rec.Repo) + " && " + a.Cfg.CodexCommand + " exec " + flags
	if resume != "" {
		inner += " resume " + execx.BashQuote(resume)
	}
	inner += " " + execx.BashQuote(prompt)
	cmd := execx.WrapScript(rec.Alias, title, inner)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := sess.RunWithStdin(runCtx, cmd, stdinR, pw, a.err())
		_ = pw.CloseWithError(err)
		errCh <- err
	}()
	dec := &render.CodexDecoder{}
	resultCh := make(chan struct{})
	var resultOnce sync.Once
	dec.OnComplete = func() { resultOnce.Do(func() { close(resultCh) }) }
	stopGrace := a.startStreamGrace(resultCh, cancel, stdinW, pr, pw)
	res := dec.Render(pr, a.out())
	stopGrace()
	parentCancel := ctx.Err() != nil
	if parentCancel || res.Err != nil {
		cancel()
		_ = stdinW.Close()
		_ = pr.Close()
		_ = pw.Close()
		a.killRemote(sess, rec.Alias, title)
	}
	runErr := waitErr(errCh, 8*time.Second)
	if parentCancel {
		return res.ThreadID, ctx.Err()
	}
	if res.Err != nil {
		return res.ThreadID, res.Err
	}
	if res.Completed {
		return res.ThreadID, nil
	}
	if runErr != nil {
		return res.ThreadID, runErr
	}
	return res.ThreadID, nil
}

func (a *App) runPlain(ctx context.Context, sess remote.Session, alias, title, inner string) error {
	cmd := execx.WrapScript(alias, title, inner)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	err := sess.RunWithStdin(runCtx, cmd, stdinR, a.out(), a.err())
	if ctx.Err() != nil {
		cancel()
		_ = stdinW.Close()
		a.killRemote(sess, alias, title)
		return ctx.Err()
	}
	return err
}

func (a *App) stop(ctx context.Context, req parse.Request) error {
	rec, err := a.Reg.Get(req.Alias)
	if err != nil {
		return err
	}
	sess, err := a.dial(ctx, rec.Workspace)
	if err != nil {
		return err
	}
	defer sess.Close()
	return sess.Run(ctx, execx.KillScript(rec.Alias, req.Title), a.out(), a.err())
}

func (a *App) refuseIfRunning(ctx context.Context, sess remote.Session, alias, title string) error {
	out, err := sess.Output(ctx, execx.IsRunningScript(alias, title))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == execx.RunStatusActive {
			return fmt.Errorf("ai run already in progress; stop it with: pcode %s stop %s", alias, title)
		}
	}
	return nil
}

func (a *App) killRemote(sess remote.Session, alias, title string) {
	kctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = sess.Run(kctx, execx.KillScript(alias, title), io.Discard, io.Discard)
}

func (a *App) startStreamGrace(notify <-chan struct{}, cancel context.CancelFunc, stdinW *io.PipeWriter, pr *io.PipeReader, pw *io.PipeWriter) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-notify:
		case <-done:
			return
		}
		grace := a.Cfg.StreamGrace
		if grace <= 0 {
			grace = 10 * time.Second
		}
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-timer.C:
			cancel()
			_ = stdinW.Close()
			_ = pr.Close()
			_ = pw.Close()
		case <-done:
		}
	}()
	return func() {
		select {
		case <-done:
		default:
			close(done)
		}
	}
}

func waitErr(ch <-chan error, d time.Duration) error {
	select {
	case err := <-ch:
		return err
	case <-time.After(d):
		return fmt.Errorf("timed out waiting for remote session to exit")
	}
}

func (a *App) dial(ctx context.Context, workspace string) (remote.Session, error) {
	if a.Dial != nil {
		return a.Dial(ctx, workspace)
	}
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	return client.Dial(ctx, workspace)
}

func (a *App) client() (remote.Client, error) {
	if a.Client != nil {
		return a.Client, nil
	}
	client, err := remote.Connect(a.Cfg.StartTimeout, a.Cfg.StartPoll)
	if err != nil {
		return nil, err
	}
	a.Client = client
	return client, nil
}

func (a *App) out() io.Writer {
	if a.Stdout != nil {
		return a.Stdout
	}
	return os.Stdout
}

func (a *App) err() io.Writer {
	if a.Stderr != nil {
		return a.Stderr
	}
	return os.Stderr
}
