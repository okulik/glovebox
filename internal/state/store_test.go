package state

import (
	"path/filepath"
	"testing"

	"github.com/okulik/glovebox/internal/manifest"
)

func TestStore_RoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}
	if saveErr := s.Save("p1", m, "applied", "ok"); saveErr != nil {
		t.Fatal(saveErr)
	}

	s2, err := Open(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := s2.Get("p1")
	if !ok {
		t.Fatal("missing record")
	}
	if rec.Manifest.Services["redis"].Image != "redis:7-alpine" {
		t.Errorf("manifest not round-tripped")
	}
	if rec.LastApply.Status != "applied" {
		t.Errorf("status = %q", rec.LastApply.Status)
	}
}

func TestStore_ProposedRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}
	yml := "version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n"
	if perr := s.SaveProposed("p1", m, yml); perr != nil {
		t.Fatal(perr)
	}

	// Persists across reopen.
	s2, err := Open(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := s2.Get("p1")
	if !ok || rec.Proposed == nil {
		t.Fatal("proposed not persisted")
	}
	if rec.ProposedYAML != yml {
		t.Errorf("ProposedYAML = %q", rec.ProposedYAML)
	}

	// Clear removes both the parsed proposal and its YAML.
	if cerr := s2.ClearProposed("p1"); cerr != nil {
		t.Fatal(cerr)
	}
	rec2, _ := s2.Get("p1")
	if rec2.Proposed != nil || rec2.ProposedYAML != "" {
		t.Errorf("proposed not cleared: %+v", rec2)
	}
}

func TestStore_SaveAppliedStoresYAML(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}
	yml := "version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n"
	if serr := s.SaveApplied("p1", m, yml, "applied", ""); serr != nil {
		t.Fatal(serr)
	}
	rec, ok := s.Get("p1")
	if !ok || rec.ManifestYAML != yml {
		t.Errorf("ManifestYAML = %q", rec.ManifestYAML)
	}
	if rec.LastApply.Status != "applied" {
		t.Errorf("status = %q", rec.LastApply.Status)
	}

	// Also verify on-disk persistence.
	s2, err := Open(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	rec2, ok2 := s2.Get("p1")
	if !ok2 || rec2.ManifestYAML != yml {
		t.Errorf("ManifestYAML not persisted: %q", rec2.ManifestYAML)
	}
}
