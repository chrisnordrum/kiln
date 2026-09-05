package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"kiln/internal/ast"
)

// Persistence is a snapshot the caller asks for, never a behaviour the store
// has. `kiln test` and `kiln snap` depend on starting empty — a test that
// inherited yesterday's rows is not a test, and a snapshot that did is not a
// regression check — so nothing here runs unless a command opts in. Only
// `kiln dev` does.
//
// The format is plain JSON, one object per row, because the file is meant to
// be opened and read while developing. That costs the type information: JSON
// has one number type and no timestamp. Rather than tag every value, the
// schema is used to put the types back on the way in — it already knows that
// Task.created is `at` and Task.project is `ref`, and it is the spec either
// way. A file that disagrees with the schema loses the argument.
const snapshotVersion = 1

type snapshot struct {
	Version int                         `json:"version"`
	Now     time.Time                   `json:"now"`
	NextID  map[string]int64            `json:"next_id"`
	Tables  map[string][]map[string]any `json:"tables"`
}

// Snapshot encodes the store as JSON.
func (s *Store) Snapshot() ([]byte, error) {
	snap := snapshot{
		Version: snapshotVersion,
		Now:     s.Now,
		NextID:  map[string]int64{},
		Tables:  map[string][]map[string]any{},
	}
	for table, id := range s.nextID {
		snap.NextID[table] = id
	}
	// Every table the schema declares gets a key, empty or not, so the file
	// shows the shape of the program rather than only what happens to be
	// populated.
	for _, t := range s.p.Tables {
		rows := make([]map[string]any, 0, len(s.tables[t.Name]))
		for _, r := range s.tables[t.Name] {
			out := make(map[string]any, len(r))
			for k, v := range r {
				out[k] = v
			}
			rows = append(rows, out)
		}
		snap.Tables[t.Name] = rows
	}
	return json.MarshalIndent(&snap, "", "  ")
}

// Restore replaces the store's contents with a snapshot's.
//
// It reads through the schema, not the file: a column the program no longer
// declares is dropped, and one it has gained since takes its declared default.
// A dev server whose schema is edited between runs keeps its data instead of
// refusing to start, which is the whole reason to persist it.
func (s *Store) Restore(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	// Without this every number arrives as float64 and an id past 2^53 comes
	// back a different id.
	dec.UseNumber()
	var snap snapshot
	if err := dec.Decode(&snap); err != nil {
		return fmt.Errorf("not a kiln data file: %w", err)
	}
	if snap.Version != snapshotVersion {
		return fmt.Errorf("data file is version %d, this kiln writes %d", snap.Version, snapshotVersion)
	}

	now := snap.Now
	if now.IsZero() {
		now = s.Now
	}
	tables := map[string][]Row{}
	for _, t := range s.p.Tables {
		for i, raw := range snap.Tables[t.Name] {
			row := Row{}
			for _, f := range t.Fields {
				v, present := raw[f.Name]
				if !present || v == nil {
					row[f.Name] = declaredDefault(f, now)
					continue
				}
				val, err := decodeValue(v, f.Type)
				if err != nil {
					return fmt.Errorf("%s row %d, field %s: %w", t.Name, i+1, f.Name, err)
				}
				row[f.Name] = val
			}
			tables[t.Name] = append(tables[t.Name], row)
		}
	}

	nextID := map[string]int64{}
	for _, t := range s.p.Tables {
		nextID[t.Name] = snap.NextID[t.Name]
		// A counter behind the rows it is meant to be ahead of would hand out
		// an id that already exists.
		for _, r := range tables[t.Name] {
			if id, ok := r["id"].(int64); ok && id > nextID[t.Name] {
				nextID[t.Name] = id
			}
		}
	}

	s.tables = tables
	s.nextID = nextID
	s.Now = now
	return nil
}

// decodeValue puts the declared type back on a value JSON flattened.
func decodeValue(raw any, kind string) (Value, error) {
	// UseNumber keeps the digits exactly as written; Coerce parses them.
	if n, ok := raw.(json.Number); ok {
		raw = n.String()
	}
	switch kind {
	case "id":
		kind = "int"
	case "enum":
		kind = "text"
	}
	return Coerce(raw, kind)
}

// declaredDefault is the value a column takes when a row does not supply one.
func declaredDefault(f *ast.Field, now time.Time) Value {
	switch {
	case f.DefaultNow:
		return now
	case f.Default != nil:
		return literal(f.Default)
	}
	return nil
}
