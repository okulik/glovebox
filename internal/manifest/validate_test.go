package manifest

import (
	"strings"
	"testing"
)

func TestParse_AcceptsValidV1(t *testing.T) {
	yml := `
version: 1
services:
  neo4j:
    image: neo4j:5.20
`
	m, err := Parse([]byte(yml), defaultRules())
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if m.Version != 1 {
		t.Errorf("version = %d, want 1", m.Version)
	}
	if _, ok := m.Services["neo4j"]; !ok {
		t.Errorf("missing service neo4j")
	}
}

func TestParse_RejectsWrongVersion(t *testing.T) {
	yml := `
version: 2
services: {}
`
	_, err := Parse([]byte(yml), defaultRules())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error does not mention version: %v", err)
	}
}

func TestParse_RejectsMissingTag(t *testing.T) {
	yml := `
version: 1
services:
  neo4j:
    image: neo4j
`
	_, err := Parse([]byte(yml), defaultRules())
	if err == nil || !strings.Contains(err.Error(), "tag") {
		t.Fatalf("expected tag-required error, got %v", err)
	}
}

func TestParse_RejectsLatestTag(t *testing.T) {
	yml := `
version: 1
services:
  neo4j:
    image: neo4j:latest
`
	_, err := Parse([]byte(yml), defaultRules())
	if err == nil || !strings.Contains(err.Error(), ":latest") {
		t.Fatalf("expected :latest rejection, got %v", err)
	}
}

func TestParse_RejectsUnknownRegistry(t *testing.T) {
	yml := `
version: 1
services:
  thing:
    image: sketchy.example.com/foo:1.0
`
	_, err := Parse([]byte(yml), defaultRules())
	if err == nil || !strings.Contains(err.Error(), "registry") {
		t.Fatalf("expected registry rejection, got %v", err)
	}
}

func TestParse_AcceptsAllowlistedRegistry(t *testing.T) {
	yml := `
version: 1
services:
  thing:
    image: ghcr.io/foo/bar:2.0
`
	if _, err := Parse([]byte(yml), defaultRules()); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestParse_AcceptsNamedVolume(t *testing.T) {
	yml := `
version: 1
services:
  db:
    image: postgres:16
    volumes:
      data: /var/lib/postgresql/data
`
	if _, err := Parse([]byte(yml), defaultRules()); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestParse_RejectsHostPathVolumeKey(t *testing.T) {
	yml := `
version: 1
services:
  db:
    image: postgres:16
    volumes:
      "/host/path": /data
`
	_, err := Parse([]byte(yml), defaultRules())
	if err == nil {
		t.Fatal("expected rejection of host-path key")
	}
}

func TestParse_RejectsRelativeContainerPath(t *testing.T) {
	yml := `
version: 1
services:
  db:
    image: postgres:16
    volumes:
      data: relative/path
`
	_, err := Parse([]byte(yml), defaultRules())
	if err == nil {
		t.Fatal("expected rejection of relative container path")
	}
}

func TestParse_AcceptsResourceWithinCaps(t *testing.T) {
	yml := `
version: 1
services:
  db:
    image: postgres:16
    resources:
      cpus: "2.0"
      memory: "2g"
`
	if _, err := Parse([]byte(yml), defaultRules()); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestParse_RejectsResourceOverCap(t *testing.T) {
	yml := `
version: 1
services:
  db:
    image: postgres:16
    resources:
      cpus: "8.0"
      memory: "16g"
`
	_, err := Parse([]byte(yml), defaultRules())
	if err == nil {
		t.Fatal("expected resource cap rejection")
	}
}

func TestParse_AcceptsLiteralEnvValue(t *testing.T) {
	yml := `
version: 1
services:
  db:
    image: postgres:16
    env:
      POSTGRES_PASSWORD: "literal"
`
	if _, err := Parse([]byte(yml), defaultRules()); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestParse_RejectsUnallowlistedEnvReference(t *testing.T) {
	yml := `
version: 1
services:
  db:
    image: postgres:16
    env:
      POSTGRES_PASSWORD: "${HOST_SECRET}"
`
	_, err := Parse([]byte(yml), defaultRules())
	if err == nil {
		t.Fatal("expected rejection of unallowlisted ${HOST_SECRET}")
	}
}

func TestParse_AcceptsAllowlistedEnvReference(t *testing.T) {
	rules := defaultRules()
	rules.AllowedEnvVars = map[string]struct{}{"PUBLIC_KEY": {}}
	yml := `
version: 1
services:
  db:
    image: postgres:16
    env:
      PUBKEY: "${PUBLIC_KEY}"
`
	if _, err := Parse([]byte(yml), rules); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestParse_ReturnsStructuredError(t *testing.T) {
	yml := `
version: 1
services:
  thing:
    image: sketchy.example.com/foo:1.0
`
	_, err := Parse([]byte(yml), defaultRules())
	verr := AsValidationError(err)
	if verr == nil {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if verr.Code != "image_registry_not_allowed" {
		t.Errorf("code = %q, want image_registry_not_allowed", verr.Code)
	}
	if verr.Path != "services.thing.image" {
		t.Errorf("path = %q", verr.Path)
	}
	if verr.HintForAgent == "" {
		t.Errorf("hint_for_agent should be populated")
	}
}

func TestParse_AcceptsAllowlistedCapAdd(t *testing.T) {
	yml := `
version: 1
services:
  db:
    image: neo4j:5.20
    cap_add: [IPC_LOCK]
`
	m, err := Parse([]byte(yml), defaultRules())
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	svc, ok := m.Services["db"]
	if !ok {
		t.Fatalf("missing service db")
	}
	if len(svc.CapAdd) != 1 || svc.CapAdd[0] != "IPC_LOCK" {
		t.Errorf("cap_add = %v, want [IPC_LOCK]", svc.CapAdd)
	}
}

func TestParse_RejectsDisallowedCapAdd(t *testing.T) {
	yml := `
version: 1
services:
  db:
    image: neo4j:5.20
    cap_add: [SYS_ADMIN]
`
	_, err := Parse([]byte(yml), defaultRules())
	verr := AsValidationError(err)
	if verr == nil {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if verr.Code != "capability_not_allowed" {
		t.Errorf("code = %q, want capability_not_allowed", verr.Code)
	}
	if verr.Path != "services.db.cap_add" {
		t.Errorf("path = %q", verr.Path)
	}
	if !strings.Contains(verr.Message, "SYS_ADMIN") {
		t.Errorf("message does not mention SYS_ADMIN: %q", verr.Message)
	}
}

func TestParse_RejectsUnknownFields(t *testing.T) {
	for _, ml := range []string{
		`version: 1
services:
  thing:
    image: redis:7-alpine
    privileged: true`,
		`version: 1
services:
  thing:
    image: redis:7-alpine
    cap_add: [SYS_ADMIN]`,
		`version: 1
services:
  thing:
    image: redis:7-alpine
    network_mode: host`,
		`version: 1
services:
  thing:
    build: .`,
	} {
		if _, err := Parse([]byte(ml), defaultRules()); err == nil {
			t.Errorf("expected rejection, got nil for:\n%s", ml)
		}
	}
}
