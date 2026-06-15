package plugin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/okulik/glovebox/internal/plugin"
)

func TestStoreListFindRemove(t *testing.T) {
	proj := t.TempDir()
	ts := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	id1, err := plugin.Store(proj, "# gbx:description: alpha\nRUN echo a\n", ts)
	if err != nil {
		t.Fatalf("Store id1: %v", err)
	}
	id2, err := plugin.Store(proj, "# gbx:description: beta\nRUN echo b\n", ts.Add(time.Second))
	if err != nil {
		t.Fatalf("Store id2: %v", err)
	}
	if id1 == id2 {
		t.Fatal("expected distinct ids")
	}

	if _, statErr := os.Stat(filepath.Join(plugin.Dir(proj), id1)); statErr != nil {
		t.Fatalf("stored file missing: %v", statErr)
	}

	list, err := plugin.List(proj)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list)=%d, want 2", len(list))
	}
	if list[0].ID > list[1].ID {
		t.Error("List is not sorted by id")
	}
	found := map[string]string{list[0].ID: list[0].Description, list[1].ID: list[1].Description}
	if found[id1] != "alpha" || found[id2] != "beta" {
		t.Errorf("descriptions wrong: %v", found)
	}

	// id1/id2 come from fixed timestamps, so their 4-char prefixes are known
	// not to collide; this exercises prefix matching without ambiguity.
	p, err := plugin.Find(proj, id1[:4])
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if p.ID != id1 {
		t.Errorf("Find returned %q, want %q", p.ID, id1)
	}

	if err := plugin.Remove(p); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(plugin.Dir(proj), id1)); !os.IsNotExist(err) {
		t.Error("file should be gone after Remove")
	}
}

func TestStoreRejectsInvalid(t *testing.T) {
	proj := t.TempDir()
	if _, err := plugin.Store(proj, "RUN true\n", time.Now()); err == nil {
		t.Error("Store should reject a fragment with no description")
	}
}

func TestListMissingDirIsEmpty(t *testing.T) {
	list, err := plugin.List(t.TempDir())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("want empty, got %d", len(list))
	}
}

func TestListIgnoresHiddenFiles(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(plugin.Dir(proj), 0o755); err != nil {
		t.Fatal(err)
	}
	// A hidden editor-draft file must not appear as a plugin.
	if err := os.WriteFile(filepath.Join(plugin.Dir(proj), ".draft-123.dockerfile"),
		[]byte("# gbx:description: x\nRUN true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := plugin.List(proj)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("hidden file should be ignored, got %d entries", len(list))
	}
}

func TestFindAmbiguousAndMissing(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(plugin.Dir(proj), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ab111111", "ab222222"} {
		if err := os.WriteFile(filepath.Join(plugin.Dir(proj), id),
			[]byte("# gbx:description: x\nRUN true\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := plugin.Find(proj, "ab"); err == nil {
		t.Error("want ambiguous error")
	}
	if _, err := plugin.Find(proj, "zz"); err == nil {
		t.Error("want no-match error")
	}
}

func TestOverwriteKeepsID(t *testing.T) {
	proj := t.TempDir()
	id, err := plugin.Store(proj, "# gbx:description: one\nRUN echo a\n", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p, err := plugin.Find(proj, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = plugin.Overwrite(p, "# gbx:description: two\nRUN echo b\n"); err != nil {
		t.Fatalf("Overwrite: %v", err)
	}
	p2, err := plugin.Find(proj, id)
	if err != nil {
		t.Fatal(err)
	}
	if p2.ID != id {
		t.Errorf("id changed: %q -> %q", id, p2.ID)
	}
	if p2.Description != "two" {
		t.Errorf("description not updated: %q", p2.Description)
	}
}

func TestOverwriteRejectsInvalid(t *testing.T) {
	proj := t.TempDir()
	id, err := plugin.Store(proj, "# gbx:description: one\nRUN echo a\n", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p, err := plugin.Find(proj, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugin.Overwrite(p, "RUN no-description\n"); err == nil {
		t.Error("Overwrite should reject an invalid fragment")
	}
}

func TestWriteDockerfile(t *testing.T) {
	proj := t.TempDir()
	if _, err := plugin.Store(proj, "# gbx:description: x\nRUN true\n", time.Now()); err != nil {
		t.Fatal(err)
	}
	plugins, err := plugin.List(proj)
	if err != nil {
		t.Fatal(err)
	}
	path, err := plugin.WriteDockerfile(proj, "glovebox-agent:local", plugins)
	if err != nil {
		t.Fatalf("WriteDockerfile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "FROM glovebox-agent:local") {
		t.Errorf("Dockerfile.plugins missing FROM: %q", data)
	}
	if filepath.Base(path) != "Dockerfile.plugins" {
		t.Errorf("unexpected path %q", path)
	}
}
