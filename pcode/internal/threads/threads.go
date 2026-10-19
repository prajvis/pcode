// SPDX-License-Identifier: Apache-2.0

package threads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var slugRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type Meta struct {
	Alias     string `json:"alias"`
	Purpose   string `json:"purpose"`
	AICLI     string `json:"ai_cli"`
	ThreadID  string `json:"thread_id"`
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	ChatID    string `json:"chat_id,omitempty"`
}

type Store struct {
	Root string
}

func Slug(value string) string {
	value = strings.TrimSpace(value)
	value = slugRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "default"
	}
	return value
}

func (s *Store) File(key, purpose string) string {
	if purpose == "" {
		purpose = "default"
	}
	return filepath.Join(s.Root, Slug(key), Slug(purpose)+".json")
}

func (s *Store) PromptFile(key, purpose string) string {
	if purpose == "" {
		purpose = "default"
	}
	return filepath.Join(s.Root, Slug(key), Slug(purpose)+".prompt.md")
}

func (s *Store) Write(key, purpose, aiCLI, workspace, repo, threadID string) error {
	path := s.File(key, purpose)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing := Meta{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &existing)
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000000Z")
	created := existing.CreatedAt
	if created == "" {
		created = now
	}
	meta := Meta{
		Alias:     key,
		Purpose:   purpose,
		AICLI:     aiCLI,
		ThreadID:  threadID,
		Workspace: workspace,
		Repo:      repo,
		CreatedAt: created,
		UpdatedAt: now,
	}
	if aiCLI == "cursor" {
		meta.ChatID = threadID
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func (s *Store) Read(key, purpose string) (Meta, error) {
	path := s.File(key, purpose)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return Meta{}, err
	}
	if meta.ThreadID == "" {
		meta.ThreadID = meta.ChatID
	}
	return meta, nil
}

func (s *Store) List(key string) ([]Meta, error) {
	dir := filepath.Join(s.Root, Slug(key))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rows []Meta
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var meta Meta
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		if meta.Purpose == "" {
			meta.Purpose = strings.TrimSuffix(entry.Name(), ".json")
		}
		if meta.ThreadID == "" {
			meta.ThreadID = meta.ChatID
		}
		rows = append(rows, meta)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].UpdatedAt > rows[j].UpdatedAt })
	return rows, nil
}

func FormatList(rows []Meta) string {
	if len(rows) == 0 {
		return "No stored local threads.\n"
	}
	var b strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&b, "%-16s ai=%s thread=%s updated=%s\n", row.Purpose, row.AICLI, row.ThreadID, row.UpdatedAt)
	}
	return b.String()
}
