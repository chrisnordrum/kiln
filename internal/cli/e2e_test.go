package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// example is the program the toolchain is built toward. It must stay canonical,
// check clean, pass its tests, and match its committed snapshots — the four
// things an agent working in this language relies on.
var example = filepath.Join("..", "..", "examples", "tasks")

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	var out, errw bytes.Buffer
	if code := Run(args, &out, &errw); code != 0 {
		t.Fatalf("kiln %s exited %d\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), code, out.String(), errw.String())
	}
	return out.String()
}

func TestExampleIsCanonical(t *testing.T) {
	if got := mustRun(t, "fmt", "--check", example); strings.TrimSpace(got) != "" {
		t.Errorf("the example is not canonical; run kiln fmt:\n%s", got)
	}
}

func TestExampleChecksClean(t *testing.T) {
	if got := mustRun(t, "check", example); strings.TrimSpace(got) != "ok" {
		t.Errorf("check said: %s", got)
	}
}

func TestExampleTestsPass(t *testing.T) {
	got := mustRun(t, "test", example)
	if strings.Contains(got, "FAIL") {
		t.Errorf("tests failed:\n%s", got)
	}
	if !strings.Contains(got, "tests passed") {
		t.Errorf("want a pass summary, got:\n%s", got)
	}
}

func TestExampleSnapshotsMatch(t *testing.T) {
	got := mustRun(t, "snap", "--check", example)
	if !strings.Contains(got, "snapshots match") {
		t.Errorf("snapshots drifted:\n%s", got)
	}
}

// A project directory with no Kiln files should say so plainly rather than
// failing in a way that sends an agent looking for a bug.
func TestMissingProjectIsExplained(t *testing.T) {
	var out, errw bytes.Buffer
	if code := Run([]string{"check", t.TempDir()}, &out, &errw); code == 0 {
		t.Fatal("want a non-zero exit for a directory with no project")
	}
	if !strings.Contains(errw.String(), "no Kiln project") {
		t.Errorf("want a clear message, got: %s", errw.String())
	}
}
