// SPDX-License-Identifier: Apache-2.0

package threads

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReadSameTitle(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	if err := store.Write("website", "review", "cursor", "ws", "/repo", "abc-123"); err != nil {
		t.Fatal(err)
	}
	meta, err := store.Read("website", "review")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ThreadID != "abc-123" {
		t.Fatalf("%+v", meta)
	}
	path := store.File("website", "review")
	if filepath.Base(path) != "review.json" {
		t.Fatalf("path=%s", path)
	}
}

func TestThreadsPerCLI(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	if err := store.Write("web", "fix", "cursor", "ws", "/repo", "cursor-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Write("web", "fix", "pi", "ws", "/repo", "pi-1"); err != nil {
		t.Fatal(err)
	}
	meta, err := store.Read("web", "fix")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ThreadFor("cursor") != "cursor-1" || meta.ThreadFor("pi") != "pi-1" || meta.ThreadFor("codex") != "" {
		t.Fatalf("%+v", meta)
	}
	if meta.AICLI != "pi" || meta.ThreadID != "pi-1" {
		t.Fatalf("latest write should be mirrored: %+v", meta)
	}
	out := FormatList([]Meta{meta})
	if !strings.Contains(out, "ai=cursor thread=cursor-1") || !strings.Contains(out, "ai=pi thread=pi-1") {
		t.Fatalf("list: %s", out)
	}
}

func TestThreadForLegacyFile(t *testing.T) {
	if (Meta{ThreadID: "old"}).ThreadFor("cursor") != "old" {
		t.Fatal("legacy file without ai_cli is a cursor chat")
	}
	if (Meta{ThreadID: "old", AICLI: "codex"}).ThreadFor("pi") != "" {
		t.Fatal("codex id must not resume pi")
	}
}
