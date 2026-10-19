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
