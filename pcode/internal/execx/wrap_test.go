// SPDX-License-Identifier: Apache-2.0

package execx

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWrapScriptText(t *testing.T) {
	script := WrapScript("web", "review this", "cd /repo && agent -p 'hi'")
	for _, want := range []string{
		`runfile="$HOME/.pcode/runs/web-review-this.pid"`,
		`mkdir -p "$HOME/.pcode/runs"`,
		"setsid bash -lc",
		"cat <&3 >/dev/null",
		"3<&-",
		"pcode_kill_tree",
		`wait "$pid"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("missing %q in:\n%s", want, script)
		}
	}
	if !strings.Contains(script, BashQuote("cd /repo && agent -p 'hi'")) {
		t.Fatalf("inner command not quoted:\n%s", script)
	}
}

func TestRunFileName(t *testing.T) {
	if got := RunFileName("web", "review this"); got != "web-review-this.pid" {
		t.Fatalf("got %q", got)
	}
	if got := RunFileName("web", ""); got != "web-default.pid" {
		t.Fatalf("got %q", got)
	}
}

func TestIsRunningAndKillScriptText(t *testing.T) {
	script := IsRunningScript("web", "t")
	if !strings.Contains(script, RunStatusActive) || !strings.Contains(script, RunStatusIdle) {
		t.Fatalf("IsRunningScript: %s", script)
	}
	if strings.Contains(script, "NOT_RUNNING") || strings.Contains(script, "echo RUNNING") {
		t.Fatalf("old tokens still present: %s", script)
	}
	kill := KillScript("web", "t")
	if !strings.Contains(kill, "SIGTERM") || !strings.Contains(kill, "SIGKILL") {
		t.Fatalf("KillScript: %s", kill)
	}
}

func TestWrapScriptKillsOnStdinEOF(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not found")
	}
	home := t.TempDir()
	script := WrapScript("web", "sleep", "sleep 30")
	runFile := filepath.Join(home, ".pcode", "runs", "web-sleep.pid")

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", script)
	cmd.Stdin = pr
	cmd.Env = append(os.Environ(), "HOME="+home)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	var pid int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(runFile)
		if err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			if pid > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == 0 {
		_ = pw.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("run file not created: %s", stderr.String())
	}

	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = pr.Close()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Logf("wrapper exit err=%v stderr=%s", err, stderr.String())
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("wrapper did not exit within 3s after stdin close")
	}

	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("pid %d still alive", pid)
	}
	if err := syscall.Kill(-pid, 0); err == nil {
		t.Fatalf("process group %d still alive", pid)
	}
	if _, err := os.Stat(runFile); !os.IsNotExist(err) {
		t.Fatalf("run file still exists, err=%v", err)
	}
}

func TestIsRunningScriptExactOutput(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}
	home := t.TempDir()
	runFile := filepath.Join(home, ".pcode", "runs", "web-check.pid")
	run := func() string {
		t.Helper()
		cmd := exec.Command("bash", "-c", IsRunningScript("web", "check"))
		cmd.Env = append(os.Environ(), "HOME="+home)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("script err=%v out=%s", err, out)
		}
		return strings.TrimSpace(string(out))
	}

	if got := run(); got != "PCODE_RUN_IDLE" {
		t.Fatalf("no run file: got %q", got)
	}

	sleep := exec.Command("sleep", "30")
	if err := sleep.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = sleep.Process.Kill()
		_, _ = sleep.Process.Wait()
	}()
	if err := os.MkdirAll(filepath.Dir(runFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runFile, []byte(strconv.Itoa(sleep.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := run(); got != "PCODE_RUN_ACTIVE" {
		t.Fatalf("live pid: got %q", got)
	}

	_ = sleep.Process.Kill()
	_, _ = sleep.Process.Wait()
	if err := os.WriteFile(runFile, []byte(strconv.Itoa(sleep.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := run(); got != "PCODE_RUN_IDLE" {
		t.Fatalf("dead pid: got %q", got)
	}
}

func TestWrapScriptStdoutReachesEOF(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not found")
	}
	home := t.TempDir()
	script := WrapScript("web", "echo", "echo hi")

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "-c", script)
	cmd.Stdin = stdinR
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW
	cmd.Env = append(os.Environ(), "HOME="+home)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = stdinR.Close()
	_ = stdoutW.Close()
	_ = stderrW.Close()
	defer func() {
		_ = stdinW.Close()
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Process.Kill()
		}
	}()

	readDone := make(chan error, 2)
	go func() {
		_, err := io.Copy(io.Discard, stdoutR)
		readDone <- err
	}()
	go func() {
		_, err := io.Copy(io.Discard, stderrR)
		readDone <- err
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	deadline := time.After(2 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case err := <-readDone:
			if err != nil {
				t.Fatalf("read: %v", err)
			}
		case <-deadline:
			t.Fatal("stdout/stderr did not reach EOF within 2s")
		}
	}
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("wrapper exit: %v", err)
		}
	case <-deadline:
		t.Fatal("wrapper did not exit within 2s")
	}

	out, err := exec.Command("ps", "-o", "pid=,comm=", "-g", strconv.Itoa(cmd.Process.Pid)).Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			if fields[0] == strconv.Itoa(cmd.Process.Pid) {
				continue
			}
			t.Fatalf("leftover process in wrapper group: %s", line)
		}
	}
}

func TestWrapScriptKillsNewSessionChild(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not found")
	}
	home := t.TempDir()
	inner := `setsid sleep 30 & echo $! > "$HOME/child.pid"; wait`
	script := WrapScript("web", "child", inner)
	childFile := filepath.Join(home, "child.pid")
	runFile := filepath.Join(home, ".pcode", "runs", "web-child.pid")

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", script)
	cmd.Stdin = stdinR
	cmd.Env = append(os.Environ(), "HOME="+home)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = stdinR.Close()
	defer func() {
		_ = stdinW.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	var childPID int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(childFile)
		if err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			if childPID > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if childPID == 0 {
		_ = stdinW.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("child pid file not created: %s", stderr.String())
	}

	if err := stdinW.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("wrapper did not exit within 4s after stdin close stderr=%s", stderr.String())
	}
	if err := syscall.Kill(childPID, 0); err == nil {
		t.Fatalf("new-session child pid %d still alive", childPID)
	}
	if _, err := os.Stat(runFile); !os.IsNotExist(err) {
		t.Fatalf("run file still exists")
	}
}

func TestKillScriptKillsNewSessionChild(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not found")
	}
	home := t.TempDir()
	inner := `setsid sleep 30 & echo $! > "$HOME/child.pid"; wait`
	script := WrapScript("web", "killchild", inner)
	childFile := filepath.Join(home, "child.pid")

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", script)
	cmd.Stdin = stdinR
	cmd.Env = append(os.Environ(), "HOME="+home)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = stdinR.Close()
	defer func() {
		_ = stdinW.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	var childPID int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(childFile)
		if err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			if childPID > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if childPID == 0 {
		_ = stdinW.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("child pid file not created: %s", stderr.String())
	}

	kill := exec.Command("bash", "-c", KillScript("web", "killchild"))
	kill.Env = append(os.Environ(), "HOME="+home)
	killDone := make(chan error, 1)
	go func() { killDone <- kill.Run() }()
	select {
	case err := <-killDone:
		if err != nil {
			t.Fatalf("KillScript: %v", err)
		}
	case <-time.After(4 * time.Second):
		_ = kill.Process.Kill()
		t.Fatal("KillScript did not finish within 4s")
	}
	if err := syscall.Kill(childPID, 0); err == nil {
		t.Fatalf("new-session child pid %d still alive after KillScript", childPID)
	}
	_ = stdinW.Close()
	_ = cmd.Wait()
}
