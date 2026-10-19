// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpsertAndGet(t *testing.T) {
	dir := t.TempDir()
	reg := Open(filepath.Join(dir, "workspaces.tsv"))
	if err := reg.Upsert(Record{Alias: "web", Workspace: "my-website", Repo: "/home/coder/website", AICLI: "agent"}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Get("web")
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace != "my-website" || got.Repo != "/home/coder/website" || got.AICLI != "cursor" {
		t.Fatalf("got %+v", got)
	}
	if err := reg.Upsert(Record{Alias: "web", Workspace: "my-website", Repo: "/home/coder/website", AICLI: "claude"}); err != nil {
		t.Fatal(err)
	}
	list, err := reg.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	if list[0].AICLI != "claude" {
		t.Fatalf("cli=%s", list[0].AICLI)
	}
	raw, _ := os.ReadFile(reg.Path)
	if string(raw) == "" {
		t.Fatal("empty registry")
	}
}

func TestUnknownAlias(t *testing.T) {
	reg := Open(filepath.Join(t.TempDir(), "workspaces.tsv"))
	if _, err := reg.Get("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	reg := Open(filepath.Join(dir, "workspaces.tsv"))
	if err := reg.Upsert(Record{Alias: "app", Workspace: "my-app", Repo: "/home/coder/my-app"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Upsert(Record{Alias: "api", Workspace: "my-api", Repo: "/home/coder/my-api"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Remove("app"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Get("app"); err == nil {
		t.Fatal("expected unknown alias after remove")
	}
	got, err := reg.Get("api")
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace != "my-api" {
		t.Fatalf("%+v", got)
	}
	if err := reg.Remove("app"); err == nil {
		t.Fatal("expected error removing missing alias")
	}
}

func TestInvalidAlias(t *testing.T) {
	reg := Open(filepath.Join(t.TempDir(), "workspaces.tsv"))
	if err := reg.Upsert(Record{Alias: "Website", Workspace: "ws", Repo: "/tmp"}); err == nil {
		t.Fatal("expected invalid alias")
	}
}

func TestExistingSharedRegistryFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.tsv")
	raw := "# alias\tworkspace\trepo\tai_cli\n" +
		"api\tmy-api\t/home/coder/api\tcursor\n" +
		"dash\tuser-dashboard\t/home/coder/user-dashboard\tcursor\n" +
		"web\tmy-website\t/home/coder/website\tcursor\n" +
		"home\talice/home-lab\t/home/coder/home\tcursor\n" +
		"lab\talice/lab\t/home/coder/lab\tcursor\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := Open(path)
	list, err := reg.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 5 {
		t.Fatalf("len=%d", len(list))
	}
	got, err := reg.Get("web")
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace != "my-website" || got.Repo != "/home/coder/website" || got.AICLI != "cursor" {
		t.Fatalf("%+v", got)
	}
	got, err = reg.Get("home")
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace != "alice/home-lab" {
		t.Fatalf("owner/name workspace not preserved: %+v", got)
	}
}
