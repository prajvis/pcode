// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"testing"

	"github.com/prajvis/pcode/pcode/internal/remote"
)

func TestFormatTemplateListOne(t *testing.T) {
	text := FormatTemplateList([]remote.TemplateInfo{{
		Name:        "dev-workspace",
		DisplayName: "Dev workspace with Git",
		Description: "Clone a git repo into a workspace.",
		Org:         "admin",
	}})
	if !strings.Contains(text, "Only one template") {
		t.Fatalf("missing single-template hint:\n%s", text)
	}
	if !strings.Contains(text, "dev-workspace") {
		t.Fatal(text)
	}
}

func TestFormatTemplateParamsSplitsRequired(t *testing.T) {
	text := FormatTemplateParams("dev-workspace", []remote.TemplateParam{
		{Name: "Git Repo", Required: true, Type: "string", Description: "clone URL"},
		{Name: "base_image", Required: false, Default: "ubuntu", Type: "string", Options: []remote.ParamOption{{Name: "ubuntu", Value: "img"}}},
	})
	if !strings.Contains(text, "REQUIRED") || !strings.Contains(text, "OPTIONAL") {
		t.Fatal(text)
	}
	req, opt, _ := strings.Cut(text, "OPTIONAL")
	if !strings.Contains(req, "Git Repo") || strings.Contains(req, "base_image") {
		t.Fatalf("required section:\n%s", req)
	}
	if !strings.Contains(opt, "base_image") {
		t.Fatalf("optional section:\n%s", opt)
	}
}
