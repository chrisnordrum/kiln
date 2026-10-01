package eval

import (
	"fmt"
	"time"

	"github.com/chrisnordrum/kiln/internal/ast"
)

// Store holds every row in the program.
type Store struct {
	p      *ast.Program
	tables map[string][]Row
	nextID map[string]int64
	// Now is the clock. It is fixed rather than read from the system so that
	// snapshots and tests are reproducible.
	Now time.Time
}

// NewStore creates an empty store for a program.
func NewStore(p *ast.Program) *Store {
	return &Store{
		p:      p,
		tables: map[string][]Row{},
		nextID: map[string]int64{},
		Now:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// All returns every row in a table.
func (s *Store) All(table string) []Row { return s.tables[table] }

// Get finds a row by id.
func (s *Store) Get(table string, id Value) (Row, bool) {
	for _, r := range s.tables[table] {
		if equal(r["id"], id) {
			return r, true
		}
	}
	return nil, false
}

// Insert adds a row, filling in the id and any declared defaults.
func (s *Store) Insert(table string, row Row) (Value, error) {
	t, ok := s.p.Table(table)
	if !ok {
		return nil, fmt.Errorf("no table %s", table)
	}
	out := Row{}
	for k, v := range row {
		out[k] = v
	}
	for _, f := range t.Fields {
		if _, given := out[f.Name]; given {
			continue
		}
		if f.Type == "id" {
			s.nextID[table]++
			out[f.Name] = s.nextID[table]
			continue
		}
		out[f.Name] = declaredDefault(f, s.Now)
	}
	// An explicit id still advances the counter, so a later insert cannot
	// collide with a seeded row.
	if id, ok := out["id"].(int64); ok && id > s.nextID[table] {
		s.nextID[table] = id
	}
	s.tables[table] = append(s.tables[table], out)
	return out["id"], nil
}

// Update sets one field of one row.
func (s *Store) Update(table string, id Value, field string, v Value) error {
	row, ok := s.Get(table, id)
	if !ok {
		return fmt.Errorf("no %s with id %v", table, Text(id))
	}
	row[field] = v
	return nil
}

// Delete removes a row and cascades to rows that reference it.
func (s *Store) Delete(table string, id Value) error {
	row, ok := s.Get(table, id)
	if !ok {
		return fmt.Errorf("no %s with id %v", table, Text(id))
	}
	s.cascade(table, row["id"])
	kept := s.tables[table][:0]
	for _, r := range s.tables[table] {
		if !equal(r["id"], id) {
			kept = append(kept, r)
		}
	}
	s.tables[table] = kept
	return nil
}

// cascade applies each declared on-delete rule to rows pointing at a deleted
// row, so the store never keeps a reference to something that is gone.
func (s *Store) cascade(table string, id Value) {
	for _, t := range s.p.Tables {
		for _, f := range t.Fields {
			if f.Type != "ref" || f.Ref != table {
				continue
			}
			switch f.OnDelete {
			case "cascade":
				var doomed []Value
				for _, r := range s.tables[t.Name] {
					if equal(r[f.Name], id) {
						doomed = append(doomed, r["id"])
					}
				}
				for _, d := range doomed {
					_ = s.Delete(t.Name, d)
				}
			case "null":
				for _, r := range s.tables[t.Name] {
					if equal(r[f.Name], id) {
						r[f.Name] = nil
					}
				}
			}
		}
	}
}

// literal converts a schema default to a runtime value.
func literal(e ast.Expr) Value {
	lit, ok := e.(*ast.Lit)
	if !ok {
		if n, ok := e.(*ast.Name); ok {
			return n.String() // an enum value
		}
		return nil
	}
	return litValue(lit)
}
