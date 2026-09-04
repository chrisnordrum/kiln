package cli

import (
	"bytes"
	"strings"
	"testing"

	"kiln/internal/docs"
)

// The reference is the spec, so a command that exists must be documented and a
// documented command must exist. Drift between the two is what sends an agent
// guessing.
func TestCommandsMatchReference(t *testing.T) {
	section, err := docs.Section("cli")
	if err != nil {
		t.Fatalf("no cli section: %v", err)
	}
	for _, name := range Names() {
		if !strings.Contains(section, "kiln "+name) {
			t.Errorf("command %q is not in the reference's cli section", name)
		}
	}
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "kiln ") {
			continue
		}
		name := strings.Fields(strings.TrimPrefix(line, "kiln "))[0]
		if _, ok := commands[name]; !ok {
			t.Errorf("reference documents %q but no such command is registered", name)
		}
	}
}

func TestDocsPrintsReference(t *testing.T) {
	var out, errw bytes.Buffer
	if code := Run([]string{"docs"}, &out, &errw); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errw.String())
	}
	if !strings.Contains(out.String(), "# Kiln reference") {
		t.Error("docs did not print the reference")
	}
}

func TestDocsSection(t *testing.T) {
	var out, errw bytes.Buffer
	if code := Run([]string{"docs", "--section", "view"}, &out, &errw); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errw.String())
	}
	got := out.String()
	if !strings.Contains(got, "§view") {
		t.Errorf("want the view section, got: %s", got)
	}
	if strings.Contains(got, "§schema") {
		t.Error("section should not bleed into the next one")
	}
}

func TestUnimplementedCommandNamesItsPhase(t *testing.T) {
	var pending string
	for _, n := range Names() {
		if commands[n].Run == nil {
			pending = n
			break
		}
	}
	if pending == "" {
		t.Skip("every command is implemented")
	}
	var out, errw bytes.Buffer
	if code := Run([]string{pending}, &out, &errw); code != 2 {
		t.Errorf("want exit 2 for %s, got %d", pending, code)
	}
	if !strings.Contains(errw.String(), "phase") {
		t.Errorf("want the phase named, got: %s", errw.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errw bytes.Buffer
	if code := Run([]string{"frobnicate"}, &out, &errw); code != 2 {
		t.Errorf("want exit 2, got %d", code)
	}
	if !strings.Contains(errw.String(), "no command") {
		t.Errorf("want a clear message, got: %s", errw.String())
	}
}
