package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every example must stay canonical, check clean, pass its tests and match its
// committed snapshots — the four things an agent working in this language
// relies on. They are discovered rather than listed, because an example that
// nothing runs is an example that quietly stops being true, and the whole
// reason a second one exists is that anything the first did not exercise was
// unverified.
func examples(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join("..", "..", "examples", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no examples found: %v", err)
	}
	return dirs
}

// eachExample runs one assertion against every example, named by its
// directory so a failure says which.
func eachExample(t *testing.T, check func(t *testing.T, dir string)) {
	t.Helper()
	for _, dir := range examples(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) { check(t, dir) })
	}
}

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
	eachExample(t, func(t *testing.T, example string) {
		if got := mustRun(t, "fmt", "--check", example); strings.TrimSpace(got) != "" {
			t.Errorf("the example is not canonical; run kiln fmt:\n%s", got)
		}
	})
}

func TestExampleChecksClean(t *testing.T) {
	eachExample(t, func(t *testing.T, example string) {
		if got := mustRun(t, "check", example); strings.TrimSpace(got) != "ok" {
			t.Errorf("check said: %s", got)
		}
	})
}

func TestExampleTestsPass(t *testing.T) {
	eachExample(t, func(t *testing.T, example string) {
		got := mustRun(t, "test", example)
		if strings.Contains(got, "FAIL") {
			t.Errorf("tests failed:\n%s", got)
		}
		if !strings.Contains(got, "tests passed") {
			t.Errorf("want a pass summary, got:\n%s", got)
		}
	})
}

func TestExampleSnapshotsMatch(t *testing.T) {
	eachExample(t, func(t *testing.T, example string) {
		got := mustRun(t, "snap", "--check", example)
		if !strings.Contains(got, "snapshots match") {
			t.Errorf("snapshots drifted:\n%s", got)
		}
	})
}

// The README's showcase route and its snapshot are quoted from the tasks
// example. They had drifted far enough that the route no longer checked —
// three parameters short — in a document whose whole claim is that this
// language does not let that happen. Each fenced block that opens like the
// example file must be that file, body for body.
func TestReadmeQuotesTheExample(t *testing.T) {
	readme := readFile(t, filepath.Join("..", "..", "README.md"))
	for _, q := range []struct{ file, skip string }{
		{"routes/project_detail.kiln", ""},
		{"snap/detail-page-lists-tasks.txt", "## /projects/1\n\n"},
	} {
		want := readFile(t, filepath.Join("..", "..", "examples", "tasks", q.file))
		if i := strings.Index(want, q.skip); q.skip != "" && i >= 0 {
			want = want[i+len(q.skip):]
		}
		first, _, _ := strings.Cut(want, "\n")
		found := false
		for _, block := range strings.Split(readme, "```\n")[1:] {
			if strings.HasPrefix(block, first+"\n") {
				found = true
				if block != want {
					t.Errorf("README quotes %s and has drifted from it; paste it in again", q.file)
				}
			}
		}
		if !found {
			t.Errorf("README no longer quotes %s, whose first line is %q", q.file, first)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
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
