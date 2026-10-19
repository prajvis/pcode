// SPDX-License-Identifier: Apache-2.0

package threads

import (
	"path/filepath"
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
