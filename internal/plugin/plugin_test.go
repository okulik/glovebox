package plugin_test

import (
	"strings"
	"testing"
	"time"

	"github.com/okulik/glovebox/internal/plugin"
)

func TestHashIDIsStableDeterministicShortHex(t *testing.T) {
	ts := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	id := plugin.HashID("RUN true\n", ts)
	if len(id) != 8 {
		t.Fatalf("len(id)=%d, want 8", len(id))
	}
	if id != strings.ToLower(id) {
		t.Errorf("id %q is not lowercase", id)
	}
	for _, r := range id {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Errorf("id %q has non-hex rune %q", id, r)
		}
	}
	if plugin.HashID("RUN true\n", ts) != id {
		t.Error("plugin.HashID is not deterministic for identical inputs")
	}
	if plugin.HashID("RUN false\n", ts) == id {
		t.Error("different content should yield a different id")
	}
	if plugin.HashID("RUN true\n", ts.Add(time.Second)) == id {
		t.Error("different timestamp should yield a different id")
	}
	// Golden value: ids end up as on-disk filenames, so the algorithm must
	// stay stable across upgrades. A change here means existing plugin ids
	// would be orphaned - treat a failure as a deliberate decision, not a typo.
	if id != "534a6c42" {
		t.Errorf("plugin.HashID golden value changed: got %q, want 534a6c42", id)
	}
}

func TestParseDescription(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"plain", "# gbx:description: does a thing\nRUN true\n", "does a thing", false},
		{"extra spaces", "#   gbx:description:   spaced   \nRUN true\n", "spaced", false},
		{"multi-line", "#   gbx:description:   spaced   \n# something else\nRUN true\n", "spaced", false},
		{"missing", "RUN true\n", "", true},
		{"empty", "# gbx:description: \nRUN true\n", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := plugin.ParseDescription(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got desc %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	good := "# gbx:description: ok\nRUN apt-get update\nENV X=1\nCOPY foo /foo\n"
	if err := plugin.Validate(good); err != nil {
		t.Errorf("good fragment rejected: %v", err)
	}
	if err := plugin.Validate("RUN true\n"); err == nil {
		t.Error("missing description should fail")
	}
	if err := plugin.Validate("# gbx:description: x\nFROM scratch\n"); err == nil {
		t.Error("FROM line should be rejected")
	}
	if err := plugin.Validate("# gbx:description: x\nADD http://x /y\n"); err == nil {
		t.Error("ADD line should be rejected")
	}
	// A commented FROM/ADD (like in the template) must be allowed.
	if err := plugin.Validate("# gbx:description: x\n# FROM glovebox-agent:local\nRUN true\n"); err != nil {
		t.Errorf("commented FROM should be allowed: %v", err)
	}
}

func TestDerivedImageTag(t *testing.T) {
	if got := plugin.DerivedImageTag("glovebox-agent:local", "abc123"); got != "glovebox-agent-abc123:local" {
		t.Errorf("got %q", got)
	}
	if got := plugin.DerivedImageTag("glovebox-agent", "abc123"); got != "glovebox-agent-abc123" {
		t.Errorf("no-tag base: got %q", got)
	}
	// A registry host with a port must not be mistaken for the tag separator:
	// only the LAST colon delimits the tag.
	if got := plugin.DerivedImageTag("registry:5000/glovebox-agent:local", "abc123"); got != "registry:5000/glovebox-agent-abc123:local" {
		t.Errorf("registry-with-port base: got %q", got)
	}
}

func TestGenerateDockerfile(t *testing.T) {
	plugins := []plugin.Plugin{
		{ID: "aaaa1111", Description: "first", Content: "RUN echo a\n"},
		{ID: "bbbb2222", Description: "second", Content: "RUN echo b"}, // no trailing newline
	}
	out := plugin.GenerateDockerfile("glovebox-agent:local", plugins)
	if !strings.HasPrefix(out, "# Generated") {
		t.Errorf("missing generated banner: %q", out)
	}
	if !strings.Contains(out, "FROM glovebox-agent:local\n") {
		t.Error("missing FROM line")
	}
	if !strings.Contains(out, "# plugin aaaa1111: first\nRUN echo a\n") {
		t.Errorf("first fragment not rendered: %q", out)
	}
	if !strings.Contains(out, "# plugin bbbb2222: second\nRUN echo b\n") {
		t.Errorf("second fragment not newline-terminated: %q", out)
	}
}

func TestTemplateContainsRequiredDirectiveAndIsInvalidUntilFilled(t *testing.T) {
	tpl := plugin.Template("abc123def456")
	// Assert against the literal directive; the constant is unexported and this
	// is a black-box (plugin_test) package.
	if !strings.Contains(tpl, "# gbx:description:") {
		t.Error("template missing the description directive")
	}
	// Out of the box the directive is empty, so the template must NOT validate.
	if err := plugin.Validate(tpl); err == nil {
		t.Error("unfilled template should fail validation")
	}
}
