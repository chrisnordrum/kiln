package eval

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// seeded builds a store with one row of every type the schema declares.
func seeded(t *testing.T) *Store {
	t.Helper()
	e := New(program(t, ""))
	if _, err := e.S.Insert("User", Row{"name": "Ada"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.S.Insert("Task", Row{
		"owner": int64(1),
		"title": "Ship it",
		"rank":  int64(3),
	}); err != nil {
		t.Fatal(err)
	}
	return e.S
}

func TestSnapshotRoundTripsEveryType(t *testing.T) {
	before := seeded(t)
	data, err := before.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	after := NewStore(before.p)
	if err := after.Restore(data); err != nil {
		t.Fatalf("restore: %v", err)
	}

	task := after.All("Task")
	if len(task) != 1 {
		t.Fatalf("got %d tasks, want 1", len(task))
	}
	row := task[0]
	// The types matter more than the values: JSON has one number type and no
	// timestamp, so this is the assertion that persistence is lossless.
	for field, want := range map[string]any{
		"id":      int64(1),
		"owner":   int64(1),
		"title":   "Ship it",
		"done":    false,
		"rank":    int64(3),
		"created": before.Now,
	} {
		if got := row[field]; got != want {
			t.Errorf("Task.%s = %#v (%T), want %#v (%T)", field, got, got, want, want)
		}
	}
	if !after.Now.Equal(before.Now) {
		t.Errorf("clock = %v, want %v", after.Now, before.Now)
	}
}

func TestRestoreKeepsTheIDCounterAheadOfTheRows(t *testing.T) {
	before := seeded(t)
	data, err := before.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	after := NewStore(before.p)
	if err := after.Restore(data); err != nil {
		t.Fatal(err)
	}
	id, err := after.Insert("Task", Row{"owner": int64(1), "title": "Next", "rank": int64(4)})
	if err != nil {
		t.Fatal(err)
	}
	if id != int64(2) {
		t.Fatalf("next id = %v, want 2 — a restored counter handed out a live id", id)
	}
	if _, ok := after.Get("Task", int64(1)); !ok {
		t.Error("the restored row was overwritten by the new one")
	}
}

// A dev server is edited while it holds data. Neither direction of schema
// drift should cost the operator the file.
func TestRestoreToleratesSchemaDrift(t *testing.T) {
	data, err := seeded(t).Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	// Two drifts at once: the file carries `note`, which the schema does not
	// declare, and has lost `rank` and `done`, which it does.
	widened := NewStore(program(t, ""))
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	tables := raw["tables"].(map[string]any)
	rows := tables["Task"].([]any)
	delete(rows[0].(map[string]any), "rank")
	delete(rows[0].(map[string]any), "done")
	rows[0].(map[string]any)["note"] = "a column this schema does not declare"
	edited, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	if err := widened.Restore(edited); err != nil {
		t.Fatalf("restore refused a drifted file: %v", err)
	}
	row := widened.All("Task")[0]
	if row["rank"] != nil {
		t.Errorf("a column the file omits should read nil, got %#v", row["rank"])
	}
	if _, ok := row["note"]; ok {
		t.Error("a column the schema does not declare was kept")
	}
	if row["done"] != false {
		t.Errorf("done = %#v, want its declared default of false", row["done"])
	}
}

func TestRestoreRejectsAFutureVersion(t *testing.T) {
	s := NewStore(program(t, ""))
	err := s.Restore([]byte(`{"version": 99, "tables": {}}`))
	if err == nil {
		t.Fatal("restored a version this kiln does not write")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error should name the version, got %q", err)
	}
}

func TestRestoreNamesTheRowItCouldNotRead(t *testing.T) {
	s := NewStore(program(t, ""))
	err := s.Restore([]byte(`{"version": 1, "tables": {"Task": [
	  {"id": 1, "title": "ok", "rank": 1},
	  {"id": 2, "title": "bad", "rank": "not a number"}
	]}}`))
	if err == nil {
		t.Fatal("restored a row whose field does not fit its column")
	}
	// A file is edited by hand, so the error has to say which row.
	for _, want := range []string{"Task", "row 2", "rank"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestSnapshotIsReadable(t *testing.T) {
	data, err := seeded(t).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	// Indented, and timestamps stay timestamps rather than becoming epochs.
	if !strings.Contains(text, "\n  \"tables\": {") {
		t.Error("the file is not indented; it is meant to be opened and read")
	}
	if !strings.Contains(text, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)) {
		t.Error("timestamps should persist as RFC3339")
	}
	// Every declared table appears, so the file shows the program's shape.
	for _, table := range []string{"User", "Task"} {
		if !strings.Contains(text, `"`+table+`"`) {
			t.Errorf("table %q is missing from the file", table)
		}
	}
}
