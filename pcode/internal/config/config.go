// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var aliasRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// prajvisRuntimeHome is the historical shared-root default. Tests override it.
var prajvisRuntimeHome = "/shared/.prajvis-coder"

type Config struct {
	Home             string
	BinDir           string
	DataDir          string
	RegistryFile     string
	DefaultAICLI     string
	AIStateDir       string
	CoderBin         string
	CursorCommand    string
	CodexCommand     string
	ClaudeCommand    string
	StartTimeout     time.Duration
	StartPoll        time.Duration
	SSHReadyAttempts int
	IdleTimeout      time.Duration
	CursorStream     bool
	CursorForce      bool
	CodexForce       bool
}

func Load() Config {
	home := resolveHome()
	dataDir := firstNonEmpty(os.Getenv("CODER_RUNTIME_DATA_DIR"), filepath.Join(home, "data"))
	binDir := firstNonEmpty(os.Getenv("CODER_RUNTIME_BIN"), filepath.Join(home, "bin"))
	registry := firstNonEmpty(
		os.Getenv("CODER_WORKSPACE_REFERENCE_FILE"),
		os.Getenv("CODER_WORKSPACE_REGISTRY_FILE"),
		filepath.Join(dataDir, "workspaces.tsv"),
	)
	aiState := os.Getenv("CODER_AI_STATE_DIR")
	if aiState == "" {
		if isWritableDir("/project") {
			aiState = "/project/.coder-ai-state"
		} else {
			aiState = filepath.Join(userHome(), ".coder-ai-state")
		}
	}
	return Config{
		Home:             home,
		BinDir:           binDir,
		DataDir:          dataDir,
		RegistryFile:     registry,
		DefaultAICLI:     firstNonEmpty(os.Getenv("CODER_DEFAULT_AI_CLI"), "cursor"),
		AIStateDir:       aiState,
		CoderBin:         firstNonEmpty(os.Getenv("CODER_BIN"), "coder"),
		CursorCommand:    firstNonEmpty(os.Getenv("CODER_AI_CURSOR_COMMAND"), "agent"),
		CodexCommand:     firstNonEmpty(os.Getenv("CODER_AI_CODEX_COMMAND"), "codex"),
		ClaudeCommand:    firstNonEmpty(os.Getenv("CODER_AI_CLAUDE_COMMAND"), "claude"),
		StartTimeout:     durationSecs("CODER_WORKSPACE_START_TIMEOUT_SECS", 300),
		StartPoll:        durationSecs("CODER_WORKSPACE_START_POLL_SECS", 3),
		SSHReadyAttempts: intEnv("CODER_SSH_READY_ATTEMPTS", 3),
		IdleTimeout:      durationSecs("CURSOR_AGENT_IDLE_TIMEOUT_SECS", 300),
		CursorStream:     os.Getenv("CURSOR_AGENT_STREAM") != "0",
		CursorForce:      os.Getenv("CURSOR_AGENT_FORCE") != "0",
		CodexForce:       os.Getenv("CODER_AI_CODEX_FORCE") != "0",
	}
}

func resolveHome() string {
	if v := strings.TrimSpace(os.Getenv("CODER_RUNTIME_HOME")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("PCODE_HOME")); v != "" {
		return v
	}
	if dirExists(prajvisRuntimeHome) {
		return prajvisRuntimeHome
	}
	return filepath.Join(xdgConfigDir(), "pcode")
}

func xdgConfigDir() string {
	if v := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); v != "" {
		return v
	}
	return filepath.Join(userHome(), ".config")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func ValidAlias(alias string) bool {
	return aliasRe.MatchString(alias)
}

func NormalizeAICLI(cli string) string {
	switch strings.TrimSpace(cli) {
	case "", "cursor", "agent", "cursor-agent":
		return "cursor"
	case "claude", "claude-code":
		return "claude"
	case "codex":
		return "codex"
	default:
		return cli
	}
}

func ThreadBacked(cli string) bool {
	switch NormalizeAICLI(cli) {
	case "cursor", "codex":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func durationSecs(key string, fallback int) time.Duration {
	return time.Duration(intEnv(key, fallback)) * time.Second
}

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func userHome() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return os.Getenv("HOME")
}

func isWritableDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	test := filepath.Join(path, ".pcode-write-test")
	if err := os.WriteFile(test, []byte("ok"), 0o600); err != nil {
		return false
	}
	_ = os.Remove(test)
	return true
}
