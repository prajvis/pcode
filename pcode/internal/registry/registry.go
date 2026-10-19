// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/prajvis/pcode/pcode/internal/config"
)

type Record struct {
	Alias     string
	Workspace string
	Repo      string
	AICLI     string
}

type Registry struct {
	Path string
}

func Open(path string) *Registry {
	return &Registry{Path: path}
}

func (r *Registry) Ensure() error {
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(r.Path); err == nil {
		return nil
	}
	return os.WriteFile(r.Path, []byte("# alias\tworkspace\trepo\tai_cli\n"), 0o644)
}

func (r *Registry) List() ([]Record, error) {
	if err := r.Ensure(); err != nil {
		return nil, err
	}
	f, err := os.Open(r.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		rec, ok := parseLine(line)
		if ok {
			out = append(out, rec)
		}
	}
	return out, scanner.Err()
}

func (r *Registry) Get(alias string) (Record, error) {
	records, err := r.List()
	if err != nil {
		return Record{}, err
	}
	for _, rec := range records {
		if rec.Alias == alias {
			return rec, nil
		}
	}
	return Record{}, fmt.Errorf("unknown alias: %s", alias)
}

func (r *Registry) Upsert(rec Record) error {
	if !config.ValidAlias(rec.Alias) {
		return fmt.Errorf("alias must match ^[a-z][a-z0-9_-]*$: %s", rec.Alias)
	}
	if rec.Workspace == "" || rec.Repo == "" {
		return fmt.Errorf("workspace and repo are required")
	}
	if rec.AICLI == "" {
		rec.AICLI = "cursor"
	}
	rec.AICLI = config.NormalizeAICLI(rec.AICLI)
	if err := r.Ensure(); err != nil {
		return err
	}

	input, err := os.ReadFile(r.Path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(input), "\n")
	found := false
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" && len(out) == len(lines)-1 {
			continue
		}
		existing, ok := parseLine(line)
		if !ok {
			if line != "" || len(out) == 0 {
				out = append(out, line)
			}
			continue
		}
		if existing.Alias == rec.Alias {
			out = append(out, format(rec))
			found = true
			continue
		}
		out = append(out, format(existing))
	}
	if !found {
		out = append(out, format(rec))
	}
	text := strings.Join(trimTrailingEmpty(out), "\n") + "\n"
	tmp := r.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.Path)
}

func (r *Registry) Remove(alias string) error {
	if alias == "" {
		return fmt.Errorf("alias is required")
	}
	if err := r.Ensure(); err != nil {
		return err
	}
	input, err := os.ReadFile(r.Path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(input), "\n")
	found := false
	var out []string
	for _, line := range lines {
		existing, ok := parseLine(line)
		if !ok {
			if line != "" || len(out) == 0 {
				out = append(out, line)
			}
			continue
		}
		if existing.Alias == alias {
			found = true
			continue
		}
		out = append(out, format(existing))
	}
	if !found {
		return fmt.Errorf("unknown alias: %s", alias)
	}
	text := strings.Join(trimTrailingEmpty(out), "\n") + "\n"
	tmp := r.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.Path)
}

func parseLine(line string) (Record, bool) {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
		return Record{}, false
	}
	parts := strings.Split(line, "\t")
	if len(parts) < 3 {
		return Record{}, false
	}
	cli := "cursor"
	if len(parts) > 3 && strings.TrimSpace(parts[3]) != "" {
		cli = parts[3]
	}
	return Record{
		Alias:     strings.TrimSpace(parts[0]),
		Workspace: strings.TrimSpace(parts[1]),
		Repo:      strings.TrimSpace(parts[2]),
		AICLI:     config.NormalizeAICLI(cli),
	}, true
}

func format(rec Record) string {
	return rec.Alias + "\t" + rec.Workspace + "\t" + rec.Repo + "\t" + rec.AICLI
}

func trimTrailingEmpty(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
