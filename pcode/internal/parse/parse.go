// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"fmt"
	"strings"

	"github.com/prajvis/pcode/pcode/internal/config"
)

type Kind string

const (
	KindHelp            Kind = "help"
	KindVersion         Kind = "version"
	KindList            Kind = "list"
	KindAdd             Kind = "add"
	KindRemove          Kind = "remove"
	KindExec            Kind = "exec"
	KindAI              Kind = "ai"
	KindShell           Kind = "shell"
	KindThreads         Kind = "threads"
	KindLogs            Kind = "logs"
	KindVerify          Kind = "verify"
	KindCreate          Kind = "create"
	KindCreateHelp      Kind = "create-help"
	KindCreateTemplates Kind = "create-templates"
	KindCreateShow      Kind = "create-show"
)

type Request struct {
	Kind       Kind
	Alias      string
	Title      string
	Prompt     string
	Command    string
	AddAlias   string
	Workspace  string
	Repo       string
	AICLI      string
	Template   string
	Org        string
	Parameters []string
}

var reserved = map[string]Kind{
	"help":    KindHelp,
	"-h":      KindHelp,
	"--help":  KindHelp,
	"version": KindVersion,
	"list":    KindList,
	"ls":      KindList,
	"add":     KindAdd,
	"remove":  KindRemove,
	"rm":      KindRemove,
	"create":  KindCreate,
}

func Parse(args []string) (Request, error) {
	if len(args) == 0 {
		return Request{Kind: KindHelp}, nil
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return Request{Kind: KindHelp}, nil
	}
	if args[0] == "version" || args[0] == "--version" {
		return Request{Kind: KindVersion}, nil
	}
	if args[0] == "create" {
		return parseCreate(args[1:])
	}
	if kind, ok := reserved[args[0]]; ok {
		if hasHelp(args[1:]) {
			return Request{Kind: KindHelp}, nil
		}
		switch kind {
		case KindAdd:
			return parseAdd(args[1:])
		case KindRemove:
			return parseRemove(args[1:])
		default:
			return Request{Kind: kind}, nil
		}
	}
	alias := args[0]
	if !config.ValidAlias(alias) {
		return Request{}, fmt.Errorf("invalid alias %q; expected pcode <alias> <exec|ai|shell|threads|logs|verify>", alias)
	}
	if len(args) == 1 || hasHelp(args[1:]) {
		return Request{Kind: KindHelp, Alias: alias}, nil
	}
	switch args[1] {
	case "exec", "x":
		if len(args) < 3 {
			return Request{}, fmt.Errorf("usage: pcode %s exec <command>", alias)
		}
		return Request{Kind: KindExec, Alias: alias, Command: strings.Join(args[2:], " ")}, nil
	case "ai":
		if len(args) < 4 {
			return Request{}, fmt.Errorf("usage: pcode %s ai <chat-title> <prompt>", alias)
		}
		return Request{
			Kind:   KindAI,
			Alias:  alias,
			Title:  args[2],
			Prompt: strings.Join(args[3:], " "),
		}, nil
	case "shell":
		return Request{Kind: KindShell, Alias: alias}, nil
	case "threads":
		return Request{Kind: KindThreads, Alias: alias}, nil
	case "logs":
		return Request{Kind: KindLogs, Alias: alias}, nil
	case "verify":
		return Request{Kind: KindVerify, Alias: alias}, nil
	default:
		return Request{}, fmt.Errorf("unknown command %q; use exec, ai, shell, threads, logs, or verify", args[1])
	}
}

func parseAdd(args []string) (Request, error) {
	req := Request{Kind: KindAdd, AICLI: "cursor"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--alias":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode add: --alias needs a value")
			}
			i++
			req.AddAlias = args[i]
		case "--workspace":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode add: --workspace needs a value")
			}
			i++
			req.Workspace = args[i]
		case "--repo":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode add: --repo needs a value")
			}
			i++
			req.Repo = args[i]
		case "--ai-cli":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode add: --ai-cli needs a value")
			}
			i++
			req.AICLI = args[i]
		default:
			return Request{}, fmt.Errorf("pcode add: unknown arg %s", args[i])
		}
	}
	if req.AddAlias == "" || req.Workspace == "" || req.Repo == "" {
		return Request{}, fmt.Errorf("usage: pcode add --alias <alias> --workspace <workspace> --repo <repo> [--ai-cli cursor]")
	}
	if !config.ValidAlias(req.AddAlias) {
		return Request{}, fmt.Errorf("pcode add: alias must match ^[a-z][a-z0-9_-]*$")
	}
	req.AICLI = config.NormalizeAICLI(req.AICLI)
	return req, nil
}

func parseRemove(args []string) (Request, error) {
	req := Request{Kind: KindRemove}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--alias":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode remove: --alias needs a value")
			}
			i++
			req.AddAlias = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				return Request{}, fmt.Errorf("pcode remove: unknown arg %s", args[i])
			}
			if req.AddAlias != "" {
				return Request{}, fmt.Errorf("usage: pcode remove <alias>")
			}
			req.AddAlias = args[i]
		}
	}
	if req.AddAlias == "" {
		return Request{}, fmt.Errorf("usage: pcode remove <alias>")
	}
	if !config.ValidAlias(req.AddAlias) {
		return Request{}, fmt.Errorf("pcode remove: alias must match ^[a-z][a-z0-9_-]*$")
	}
	return req, nil
}

func parseCreate(args []string) (Request, error) {
	if len(args) == 0 || hasHelp(args) {
		return Request{Kind: KindCreateHelp}, nil
	}
	req := Request{Kind: KindCreate, AICLI: "cursor"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--templates":
			req.Kind = KindCreateTemplates
		case "--show-template":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode create: --show-template needs a template name")
			}
			i++
			req.Kind = KindCreateShow
			req.Template = args[i]
		case "--alias":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode create: --alias needs a value")
			}
			i++
			req.AddAlias = args[i]
		case "--workspace":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode create: --workspace needs a value")
			}
			i++
			req.Workspace = args[i]
		case "--template":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode create: --template needs a value")
			}
			i++
			req.Template = args[i]
		case "--repo":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode create: --repo needs a value")
			}
			i++
			req.Repo = args[i]
		case "--org":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode create: --org needs a value")
			}
			i++
			req.Org = args[i]
		case "--ai-cli":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode create: --ai-cli needs a value")
			}
			i++
			req.AICLI = args[i]
		case "--parameter":
			if i+1 >= len(args) {
				return Request{}, fmt.Errorf("pcode create: --parameter needs Name=value")
			}
			i++
			if !strings.Contains(args[i], "=") {
				return Request{}, fmt.Errorf("pcode create: --parameter must be Name=value")
			}
			req.Parameters = append(req.Parameters, args[i])
		default:
			return Request{}, fmt.Errorf("pcode create: unknown arg %s (see pcode create --help)", args[i])
		}
	}
	if req.Kind == KindCreateTemplates || req.Kind == KindCreateShow {
		return req, nil
	}
	if req.AddAlias == "" || req.Workspace == "" || req.Template == "" {
		return Request{}, fmt.Errorf("usage: pcode create --alias <alias> --workspace <name> --template <template> [--repo <path>] [--parameter Name=value]...")
	}
	if !config.ValidAlias(req.AddAlias) {
		return Request{}, fmt.Errorf("pcode create: alias must match ^[a-z][a-z0-9_-]*$")
	}
	req.AICLI = config.NormalizeAICLI(req.AICLI)
	return req, nil
}

func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}
