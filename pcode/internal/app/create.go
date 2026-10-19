// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/prajvis/pcode/pcode/internal/parse"
	"github.com/prajvis/pcode/pcode/internal/registry"
	"github.com/prajvis/pcode/pcode/internal/remote"
)

func CreateUsage() string {
	return `pcode create — make a new Coder workspace, then register the alias.

Do not guess templates or parameter values. Work with the user.

STEPS
  1. pcode create --templates
     Show the list. If more than one, ask which to use.
     If only one, tell the user you will use it unless they say otherwise.
  2. pcode create --show-template <name>
     Ask the user for every REQUIRED field.
     For OPTIONAL fields, keep the printed default unless the user wants
     something else (CPU, disk, install flags, base image, …).
  3. Agree on:
       --alias        short local name (web)
       --workspace    Coder workspace name (my-website)
       git URL, clone directory, git email, git username
       any other required params the template printed
  4. pcode create --alias <alias> --workspace <name> --template <template> \
       --repo /home/coder/<Directory Name> \
       --parameter 'Git Repo=<url>' \
       --parameter 'Directory Name=<dir>' \
       --parameter 'Git Email=<email>' \
       --parameter 'Git Username=<user>' \
       [--parameter 'Name=value']...
  5. pcode <alias> verify

COMMANDS
  pcode create --help
  pcode create --templates
  pcode create --show-template <name>
  pcode create --alias <alias> --workspace <name> --template <template>
             [--repo <abs-path>] [--org <org>] [--ai-cli cursor]
             [--parameter 'Name=value']...

--repo is the path inside the workspace. If omitted and the template has
Directory Name, pcode uses /home/coder/<Directory Name>.

Use the template the user chose from --templates. Required params depend
on that template; --show-template lists them. Common git templates need
Git Repo, Directory Name, Git Email, and Git Username. Do not invent
templates or parameter names.
`
}

func FormatTemplateList(rows []remote.TemplateInfo) string {
	if len(rows) == 0 {
		return "No templates available.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-28s %-36s %s\n", "NAME", "DISPLAY", "ORG")
	for _, row := range rows {
		display := row.DisplayName
		if display == "" {
			display = row.Name
		}
		fmt.Fprintf(&b, "%-28s %-36s %s\n", row.Name, display, row.Org)
		if strings.TrimSpace(row.Description) != "" {
			fmt.Fprintf(&b, "  %s\n", strings.TrimSpace(row.Description))
		}
	}
	if len(rows) == 1 {
		fmt.Fprintf(&b, "\nOnly one template. Use it unless the user wants something else: %s\n", rows[0].Name)
	} else {
		fmt.Fprintln(&b, "\nAsk the user which template to use, then: pcode create --show-template <name>")
	}
	return b.String()
}

func FormatTemplateParams(name string, params []remote.TemplateParam) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Template %s\n\nREQUIRED — ask the user for these\n", name)
	required := 0
	for _, p := range params {
		if !p.Required {
			continue
		}
		required++
		writeParam(&b, p)
	}
	if required == 0 {
		fmt.Fprintln(&b, "  (none)")
	}
	fmt.Fprintln(&b, "\nOPTIONAL — keep defaults unless the user cares")
	optional := 0
	for _, p := range params {
		if p.Required {
			continue
		}
		optional++
		writeParam(&b, p)
	}
	if optional == 0 {
		fmt.Fprintln(&b, "  (none)")
	}
	fmt.Fprintln(&b, "\nThen run pcode create --alias ... --workspace ... --template ... --parameter 'Name=value' ...")
	return b.String()
}

func writeParam(b *strings.Builder, p remote.TemplateParam) {
	label := p.Name
	if p.DisplayName != "" && p.DisplayName != p.Name {
		label = p.DisplayName + " (" + p.Name + ")"
	}
	fmt.Fprintf(b, "  %s", label)
	if p.Type != "" {
		fmt.Fprintf(b, "  type=%s", p.Type)
	}
	if p.Default != "" {
		fmt.Fprintf(b, "  default=%s", p.Default)
	}
	fmt.Fprintln(b)
	if p.Description != "" {
		fmt.Fprintf(b, "    %s\n", p.Description)
	}
	if len(p.Options) > 0 {
		var opts []string
		for _, o := range p.Options {
			if o.Name != "" && o.Name != o.Value {
				opts = append(opts, o.Name+"="+o.Value)
			} else {
				opts = append(opts, o.Value)
			}
		}
		fmt.Fprintf(b, "    options: %s\n", strings.Join(opts, ", "))
	}
}

func (a *App) createTemplates(ctx context.Context) error {
	client, err := a.client()
	if err != nil {
		return err
	}
	rows, err := client.Templates(ctx)
	if err != nil {
		return err
	}
	fmt.Fprint(a.out(), FormatTemplateList(rows))
	return nil
}

func (a *App) createShow(ctx context.Context, req parse.Request) error {
	client, err := a.client()
	if err != nil {
		return err
	}
	params, err := client.TemplateParams(ctx, req.Template)
	if err != nil {
		return err
	}
	fmt.Fprint(a.out(), FormatTemplateParams(req.Template, params))
	return nil
}

func (a *App) create(ctx context.Context, req parse.Request) error {
	client, err := a.client()
	if err != nil {
		return err
	}
	params := map[string]string{}
	for _, raw := range req.Parameters {
		name, value, _ := strings.Cut(raw, "=")
		params[name] = value
	}
	repo := strings.TrimSpace(req.Repo)
	if repo == "" {
		if dir := strings.TrimSpace(params["Directory Name"]); dir != "" {
			repo = path.Join("/home/coder", dir)
		}
	}
	if repo == "" {
		return fmt.Errorf("pcode create: --repo is required (or pass --parameter 'Directory Name=...')")
	}
	fmt.Fprintf(a.err(), "creating workspace %s from template %s…\n", req.Workspace, req.Template)
	if err := client.CreateWorkspace(ctx, remote.CreateSpec{
		Name:       req.Workspace,
		Template:   req.Template,
		Org:        req.Org,
		Parameters: params,
	}); err != nil {
		return err
	}
	if err := a.Reg.Upsert(registry.Record{
		Alias:     req.AddAlias,
		Workspace: req.Workspace,
		Repo:      repo,
		AICLI:     req.AICLI,
	}); err != nil {
		return err
	}
	fmt.Fprintf(a.out(), "registered %s -> %s %s (%s)\n", req.AddAlias, req.Workspace, repo, req.AICLI)
	fmt.Fprintf(a.err(), "waiting for %s to start…\n", req.Workspace)
	if err := client.EnsureRunning(ctx, req.Workspace); err != nil {
		return err
	}
	return a.verify(ctx, parse.Request{Alias: req.AddAlias})
}
