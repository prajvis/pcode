// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"fmt"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

type TemplateInfo struct {
	Name        string
	DisplayName string
	Description string
	Org         string
}

type ParamOption struct {
	Name  string
	Value string
}

type TemplateParam struct {
	Name        string
	DisplayName string
	Description string
	Type        string
	Default     string
	Required    bool
	Options     []ParamOption
}

type CreateSpec struct {
	Name       string
	Template   string
	Org        string
	Parameters map[string]string
}

func (c *SDKClient) Templates(ctx context.Context) ([]TemplateInfo, error) {
	rows, err := c.api.Templates(ctx, codersdk.TemplateFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]TemplateInfo, 0, len(rows))
	for _, t := range rows {
		if t.Deprecated || t.Deleted {
			continue
		}
		out = append(out, TemplateInfo{
			Name:        t.Name,
			DisplayName: t.DisplayName,
			Description: t.Description,
			Org:         t.OrganizationName,
		})
	}
	return out, nil
}

func (c *SDKClient) TemplateParams(ctx context.Context, name string) ([]TemplateParam, error) {
	tpl, err := c.findTemplate(ctx, name)
	if err != nil {
		return nil, err
	}
	raw, err := c.api.TemplateVersionRichParameters(ctx, tpl.ActiveVersionID)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateParam, 0, len(raw))
	for _, p := range raw {
		item := TemplateParam{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Description: strings.TrimSpace(p.DescriptionPlaintext),
			Type:        p.Type,
			Default:     p.DefaultValue,
			Required:    p.Required,
		}
		if item.Description == "" {
			item.Description = strings.TrimSpace(p.Description)
		}
		for _, opt := range p.Options {
			item.Options = append(item.Options, ParamOption{Name: opt.Name, Value: opt.Value})
		}
		out = append(out, item)
	}
	return out, nil
}

func (c *SDKClient) CreateWorkspace(ctx context.Context, spec CreateSpec) error {
	tpl, err := c.findTemplate(ctx, spec.Template)
	if err != nil {
		return err
	}
	var values []codersdk.WorkspaceBuildParameter
	for name, value := range spec.Parameters {
		values = append(values, codersdk.WorkspaceBuildParameter{Name: name, Value: value})
	}
	_, err = c.api.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
		TemplateID:          tpl.ID,
		Name:                spec.Name,
		RichParameterValues: values,
	})
	if err != nil {
		return fmt.Errorf("create workspace %q: %w", spec.Name, err)
	}
	return nil
}

func (c *SDKClient) findTemplate(ctx context.Context, name string) (codersdk.Template, error) {
	rows, err := c.api.Templates(ctx, codersdk.TemplateFilter{})
	if err != nil {
		return codersdk.Template{}, err
	}
	name = strings.TrimSpace(name)
	var matches []codersdk.Template
	for _, t := range rows {
		if t.Deprecated || t.Deleted {
			continue
		}
		if t.Name == name || t.DisplayName == name {
			matches = append(matches, t)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 && name != "" {
		for _, t := range matches {
			if t.Name == name {
				return t, nil
			}
		}
	}
	return codersdk.Template{}, fmt.Errorf("unknown template %q; run pcode create --templates", name)
}
