// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"golang.org/x/crypto/ssh"

	"github.com/prajvis/pcode/pcode/internal/execx"
)

type Session interface {
	Run(ctx context.Context, command string, stdout, stderr io.Writer) error
	Output(ctx context.Context, command string) (string, error)
	Shell(ctx context.Context) error
	Close() error
}

type Client interface {
	Whoami(ctx context.Context) (string, error)
	EnsureRunning(ctx context.Context, workspace string) error
	Dial(ctx context.Context, workspace string) (Session, error)
	Logs(ctx context.Context, workspace string) error
	Templates(ctx context.Context) ([]TemplateInfo, error)
	TemplateParams(ctx context.Context, name string) ([]TemplateParam, error)
	CreateWorkspace(ctx context.Context, spec CreateSpec) error
}

type SDKClient struct {
	api          *codersdk.Client
	startTimeout time.Duration
	startPoll    time.Duration
}

func Connect(startTimeout, startPoll time.Duration) (*SDKClient, error) {
	rawURL := strings.TrimSpace(os.Getenv("CODER_URL"))
	token := strings.TrimSpace(os.Getenv("CODER_SESSION_TOKEN"))
	configDir := strings.TrimSpace(os.Getenv("CODER_CONFIG_DIR"))
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config", "coderv2")
	}
	if rawURL == "" {
		if b, err := os.ReadFile(filepath.Join(configDir, "url")); err == nil {
			rawURL = strings.TrimSpace(string(b))
		}
	}
	if token == "" {
		if b, err := os.ReadFile(filepath.Join(configDir, "session")); err == nil {
			token = strings.TrimSpace(string(b))
		}
	}
	if rawURL == "" {
		return nil, fmt.Errorf("CODER_URL is not set and %s/url is missing; run coder login first", configDir)
	}
	if token == "" {
		return nil, fmt.Errorf("CODER_SESSION_TOKEN is not set and %s/session is missing; run coder login first", configDir)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse CODER_URL: %w", err)
	}
	api := codersdk.New(u)
	api.SetSessionToken(token)
	if startTimeout <= 0 {
		startTimeout = 5 * time.Minute
	}
	if startPoll <= 0 {
		startPoll = 3 * time.Second
	}
	return &SDKClient{api: api, startTimeout: startTimeout, startPoll: startPoll}, nil
}

func (c *SDKClient) Whoami(ctx context.Context) (string, error) {
	user, err := c.api.User(ctx, codersdk.Me)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

func (c *SDKClient) EnsureRunning(ctx context.Context, workspace string) error {
	ws, err := c.api.ResolveWorkspace(ctx, workspace)
	if err != nil {
		return fmt.Errorf("workspace %q: %w", workspace, err)
	}
	if ws.LatestBuild.Status == codersdk.WorkspaceStatusRunning {
		return nil
	}
	switch ws.LatestBuild.Status {
	case codersdk.WorkspaceStatusStopped, codersdk.WorkspaceStatusDeleted:
		if ws.LatestBuild.Status == codersdk.WorkspaceStatusDeleted {
			return fmt.Errorf("workspace %q is deleted", workspace)
		}
		fmt.Fprintf(os.Stderr, "Workspace %q is stopped. Starting it...\n", workspace)
		_, err := c.api.CreateWorkspaceBuild(ctx, ws.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionStart,
		})
		if err != nil {
			return fmt.Errorf("start workspace %q: %w", workspace, err)
		}
	default:
		fmt.Fprintf(os.Stderr, "Workspace %q is %s. Waiting...\n", workspace, ws.LatestBuild.Status)
	}
	deadline := time.Now().Add(c.startTimeout)
	for time.Now().Before(deadline) {
		ws, err = c.api.ResolveWorkspace(ctx, workspace)
		if err != nil {
			return err
		}
		switch ws.LatestBuild.Status {
		case codersdk.WorkspaceStatusRunning:
			return nil
		case codersdk.WorkspaceStatusFailed, codersdk.WorkspaceStatusCanceled, codersdk.WorkspaceStatusCanceling:
			return fmt.Errorf("workspace %q entered status %s", workspace, ws.LatestBuild.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.startPoll):
		}
	}
	return fmt.Errorf("timed out waiting for workspace %q to start", workspace)
}

func (c *SDKClient) Dial(ctx context.Context, workspace string) (Session, error) {
	if err := c.EnsureRunning(ctx, workspace); err != nil {
		return nil, err
	}
	ws, err := c.api.ResolveWorkspace(ctx, workspace)
	if err != nil {
		return nil, err
	}
	agent, err := pickAgent(ws)
	if err != nil {
		return nil, err
	}
	conn, err := workspacesdk.New(c.api).DialAgent(ctx, agent.ID, &workspacesdk.DialAgentOptions{})
	if err != nil {
		return nil, fmt.Errorf("dial agent: %w", err)
	}
	if !conn.AwaitReachable(ctx) {
		_ = conn.Close()
		return nil, fmt.Errorf("agent for %q is not reachable", workspace)
	}
	sshClient, err := conn.SSHClient(ctx)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh client: %w", err)
	}
	return &sshSess{ssh: sshClient, conn: conn}, nil
}

func (c *SDKClient) Logs(ctx context.Context, workspace string) error {
	ws, err := c.api.ResolveWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	ch, closer, err := c.api.WorkspaceBuildLogsAfter(ctx, ws.LatestBuild.ID, 0)
	if err != nil {
		return err
	}
	defer closer.Close()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case logs, ok := <-ch:
			if !ok {
				return nil
			}
			fmt.Println(logs.Output)
		}
	}
}

func pickAgent(ws codersdk.Workspace) (codersdk.WorkspaceAgent, error) {
	var agents []codersdk.WorkspaceAgent
	for _, resource := range ws.LatestBuild.Resources {
		agents = append(agents, resource.Agents...)
	}
	if len(agents) == 0 {
		return codersdk.WorkspaceAgent{}, fmt.Errorf("workspace %q has no agents", ws.Name)
	}
	if len(agents) == 1 {
		return agents[0], nil
	}
	for _, agent := range agents {
		if agent.Name == "main" {
			return agent, nil
		}
	}
	return agents[0], nil
}

type sshSess struct {
	ssh  *ssh.Client
	conn workspacesdk.AgentConn
}

func (s *sshSess) Close() error {
	err := s.ssh.Close()
	if s.conn != nil {
		_ = s.conn.Close()
	}
	return err
}

func (s *sshSess) Run(ctx context.Context, command string, stdout, stderr io.Writer) error {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdout = stdout
	sess.Stderr = stderr
	wrapped := "bash -lc " + execx.BashQuote(command)
	errCh := make(chan error, 1)
	go func() { errCh <- sess.Run(wrapped) }()
	select {
	case <-ctx.Done():
		_ = sess.Close()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *sshSess) Output(ctx context.Context, command string) (string, error) {
	var buf strings.Builder
	err := s.Run(ctx, command, &buf, io.Discard)
	return buf.String(), err
}

func (s *sshSess) Shell(ctx context.Context) error {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdin = os.Stdin
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	_ = sess.RequestPty("xterm", 80, 40, ssh.TerminalModes{})
	if err := sess.Shell(); err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- sess.Wait() }()
	select {
	case <-ctx.Done():
		_ = sess.Close()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
