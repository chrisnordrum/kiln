package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisnordrum/kiln/internal/eval"
)

// The point of persistence is that a restart is invisible, so the test is two
// servers over one file rather than a snapshot compared to itself.
func TestActionsSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")

	first := newServer(t, true)
	loaded, err := first.UseFile(path)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if loaded {
		t.Fatal("first run loaded a file that does not exist yet")
	}
	if out := post(t, first, `{"action":"toggle","args":{"id":1,"done":true},"path":"/tasks"}`); out.Error != "" {
		t.Fatalf("toggle: %s", out.Error)
	}
	if out := post(t, first, `{"action":"add","args":{"title":"Second"},"path":"/tasks"}`); out.Error != "" {
		t.Fatalf("add: %s", out.Error)
	}

	second := New(program(t))
	second.SignIn("user", int64(1))
	loaded, err = second.UseFile(path)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !loaded {
		t.Fatal("second run did not find the file the first one wrote")
	}

	row, ok := second.r.Store().Get("Task", int64(1))
	if !ok {
		t.Fatal("the seeded task did not survive")
	}
	if row["done"] != true {
		t.Errorf("done = %#v, want true — the toggle did not survive", row["done"])
	}
	if got := len(second.r.Store().All("Task")); got != 2 {
		t.Errorf("%d tasks after restart, want 2", got)
	}

	// The page a visitor gets must agree with the file, not just the store.
	body := get(t, second, "/tasks").Body.String()
	if !strings.Contains(body, "Second") {
		t.Error("the task added before the restart is not on the page after it")
	}
}

func TestARestartDoesNotReuseALiveID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	first := newServer(t, true)
	if _, err := first.UseFile(path); err != nil {
		t.Fatal(err)
	}
	if out := post(t, first, `{"action":"add","args":{"title":"Second"},"path":"/tasks"}`); out.Error != "" {
		t.Fatalf("add: %s", out.Error)
	}

	second := New(program(t))
	second.SignIn("user", int64(1))
	if _, err := second.UseFile(path); err != nil {
		t.Fatal(err)
	}
	if out := post(t, second, `{"action":"add","args":{"title":"Third"},"path":"/tasks"}`); out.Error != "" {
		t.Fatalf("add after restart: %s", out.Error)
	}

	seen := map[eval.Value]string{}
	for _, r := range second.r.Store().All("Task") {
		if prev, dup := seen[r["id"]]; dup {
			t.Fatalf("id %v belongs to both %q and %q", r["id"], prev, r["title"])
		}
		seen[r["id"]] = eval.Text(r["title"])
	}
	if len(seen) != 3 {
		t.Errorf("%d tasks, want 3", len(seen))
	}
}

func TestWithoutAFileNothingIsWritten(t *testing.T) {
	dir := t.TempDir()
	s := newServer(t, true)
	if out := post(t, s, `{"action":"add","args":{"title":"Second"},"path":"/tasks"}`); out.Error != "" {
		t.Fatalf("add: %s", out.Error)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("an in-memory server wrote %d file(s)", len(entries))
	}
}

func TestAnUnreadableFileIsRefusedByName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := newServer(t, true).UseFile(path)
	if err == nil {
		t.Fatal("started on a file it could not read")
	}
	// The operator has to know which file to go and look at.
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the file", err)
	}
}

// The file that could not be read is the only copy of the data, so a refused
// load must not leave the server saving over it.
func TestARefusedFileIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	original := "{not json"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newServer(t, true)
	if _, err := s.UseFile(path); err == nil {
		t.Fatal("started on a file it could not read")
	}
	if out := post(t, s, `{"action":"add","args":{"title":"Second"},"path":"/tasks"}`); out.Error != "" {
		t.Fatalf("add: %s", out.Error)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Errorf("the unreadable file was overwritten:\n%s", after)
	}
}

// A save writes beside the file and renames over it, so the data is never a
// half-written file.
func TestSaveLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	s := newServer(t, true)
	if _, err := s.UseFile(path); err != nil {
		t.Fatal(err)
	}
	if out := post(t, s, `{"action":"add","args":{"title":"Second"},"path":"/tasks"}`); out.Error != "" {
		t.Fatalf("add: %s", out.Error)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "store.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only store.json", names)
	}
}
