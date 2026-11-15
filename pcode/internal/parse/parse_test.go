// SPDX-License-Identifier: Apache-2.0

package parse

import "testing"

func TestParseExecAndAI(t *testing.T) {
	req, err := Parse([]string{"website", "exec", "pwd"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Kind != KindExec || req.Alias != "website" || req.Command != "pwd" {
		t.Fatalf("%+v", req)
	}

	req, err = Parse([]string{"website", "exec", "git", "status", "--short"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Command != "git status --short" {
		t.Fatalf("command=%q", req.Command)
	}

	req, err = Parse([]string{"website", "ai", "review", "look at the diff"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Kind != KindAI || req.Title != "review" || req.Prompt != "look at the diff" {
		t.Fatalf("%+v", req)
	}

	req, err = Parse([]string{"website", "ai", "review", "fix", "the", "comments"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Prompt != "fix the comments" {
		t.Fatalf("prompt=%q", req.Prompt)
	}
}

func TestParseReservedAndAdd(t *testing.T) {
	req, err := Parse([]string{"list"})
	if err != nil || req.Kind != KindList {
		t.Fatalf("%+v %v", req, err)
	}
	req, err = Parse([]string{"add", "--alias", "web", "--workspace", "my-website", "--repo", "/home/coder/website"})
	if err != nil {
		t.Fatal(err)
	}
	if req.AddAlias != "web" || req.Workspace != "my-website" || req.AICLI != "cursor" {
		t.Fatalf("%+v", req)
	}

	req, err = Parse([]string{"remove", "app"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Kind != KindRemove || req.AddAlias != "app" {
		t.Fatalf("%+v", req)
	}
	req, err = Parse([]string{"rm", "--alias", "app"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Kind != KindRemove || req.AddAlias != "app" {
		t.Fatalf("%+v", req)
	}
	if _, err := Parse([]string{"remove"}); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestParseAIRequiresTitleAndPrompt(t *testing.T) {
	if _, err := Parse([]string{"website", "ai", "review"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseHelp(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--help"},
		{"-h"},
		{"help"},
		{"add", "--help"},
		{"remove", "--help"},
		{"website", "--help"},
		{"website", "ai", "--help"},
	} {
		req, err := Parse(args)
		if err != nil {
			t.Fatalf("args=%v err=%v", args, err)
		}
		if req.Kind != KindHelp {
			t.Fatalf("args=%v kind=%s", args, req.Kind)
		}
	}
}

func TestParseCreate(t *testing.T) {
	req, err := Parse([]string{"create"})
	if err != nil || req.Kind != KindCreateHelp {
		t.Fatalf("%+v %v", req, err)
	}
	req, err = Parse([]string{"create", "--help"})
	if err != nil || req.Kind != KindCreateHelp {
		t.Fatalf("%+v %v", req, err)
	}
	req, err = Parse([]string{"create", "--templates"})
	if err != nil || req.Kind != KindCreateTemplates {
		t.Fatalf("%+v %v", req, err)
	}
	req, err = Parse([]string{"create", "--show-template", "dev-workspace"})
	if err != nil || req.Kind != KindCreateShow || req.Template != "dev-workspace" {
		t.Fatalf("%+v %v", req, err)
	}
	req, err = Parse([]string{
		"create",
		"--alias", "website",
		"--workspace", "my-website",
		"--template", "dev-workspace",
		"--parameter", "Git Repo=git@example.com:org/repo.git",
		"--parameter", "Directory Name=website",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Kind != KindCreate || req.AddAlias != "website" || len(req.Parameters) != 2 {
		t.Fatalf("%+v", req)
	}
}

func TestParseVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		req, err := Parse(args)
		if err != nil {
			t.Fatalf("args=%v err=%v", args, err)
		}
		if req.Kind != KindVersion {
			t.Fatalf("args=%v kind=%s", args, req.Kind)
		}
	}
}

func TestParseStop(t *testing.T) {
	req, err := Parse([]string{"website", "stop", "review"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Kind != "stop" || req.Alias != "website" || req.Title != "review" {
		t.Fatalf("%+v", req)
	}
	if _, err := Parse([]string{"website", "stop"}); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestParseAICLIOverride(t *testing.T) {
	for _, args := range [][]string{
		{"web", "ai", "--ai-cli", "pi", "review", "look", "here"},
		{"web", "ai", "--ai-cli=pi-agent", "review", "look", "here"},
	} {
		req, err := Parse(args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if req.AICLI != "pi" || req.Title != "review" || req.Prompt != "look here" {
			t.Fatalf("%v: %+v", args, req)
		}
	}
	req, err := Parse([]string{"web", "ai", "review", "--ai-cli", "pi"})
	if err != nil || req.AICLI != "" || req.Prompt != "--ai-cli pi" {
		t.Fatalf("flag after the title must stay prompt text: %+v %v", req, err)
	}
	for _, bad := range [][]string{
		{"web", "ai", "--ai-cli"},
		{"web", "ai", "--ai-cli", "vim", "t", "p"},
		{"web", "ai", "--ai-cli", "pi", "only-title"},
		{"web", "ai", "--ai-clix", "t", "p"},
	} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("expected error for %v", bad)
		}
	}
}

func TestParseAIModel(t *testing.T) {
	for _, args := range [][]string{
		{"web", "ai", "--model", "kimi", "--ai-cli", "pi", "t", "do", "it"},
		{"web", "ai", "--ai-cli=pi", "--model=kimi", "t", "do", "it"},
	} {
		req, err := Parse(args)
		if err != nil || req.Model != "kimi" || req.AICLI != "pi" || req.Title != "t" || req.Prompt != "do it" {
			t.Fatalf("%v: %+v %v", args, req, err)
		}
	}
	req, err := Parse([]string{"web", "ai", "--model", "gpt-5.3-codex", "t", "p"})
	if err != nil || req.Model != "gpt-5.3-codex" || req.AICLI != "" {
		t.Fatalf("model without --ai-cli: %+v %v", req, err)
	}
	req, err = Parse([]string{"web", "ai", "--", "--model-test", "p"})
	if err != nil || req.Title != "--model-test" || req.Model != "" {
		t.Fatalf("-- ends flags: %+v %v", req, err)
	}
	for _, bad := range [][]string{
		{"web", "ai", "--model"},
		{"web", "ai", "--model=", "t", "p"},
		{"web", "ai", "--model", "x", "only-title"},
		{"web", "ai", "--modelx", "t", "p"},
	} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("expected error for %v", bad)
		}
	}
}
