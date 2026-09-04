// Package project locates and loads the .kiln files that make up a program.
package project

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kiln/internal/parse"
)

// Ext is the source file extension.
const Ext = ".kiln"

// skipDirs are never searched for source.
var skipDirs = map[string]bool{
	".git": true, ".kiln": true, "node_modules": true, "snap": true, "dist": true,
}

// Load reads every .kiln file under root, sorted by path so a program's
// meaning never depends on filesystem ordering.
func Load(root string) ([]parse.File, error) {
	var out []parse.File
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if skipDirs[e.Name()] || strings.HasPrefix(e.Name(), ".") && e.Name() != "." {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != Ext {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		out = append(out, parse.File{Path: filepath.ToSlash(rel), Src: string(src)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Root finds the directory holding app.kiln, searching dir and its parents, so
// a command works from anywhere inside a project.
func Root(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, "app"+Ext)); err == nil {
			return abs, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}
