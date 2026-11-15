// Package home applies a description to a home directory, one part at a
// time.
package home

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/waco-org/hiwthi/internal/startup"
)

// ConfigDir is where Hiwthi keeps the files it owns, relative to the home.
// Fixed, not $XDG_CONFIG_HOME: the marked lines name it.
const ConfigDir = ".config/hiwthi"

// Change is one file Hiwthi created or changed.
type Change struct {
	Path string // relative to the home
	What string
}

// writeOwned writes one of the files Hiwthi owns in ConfigDir.
func writeOwned(homeDir, name, content string) ([]Change, error) {
	rel := filepath.Join(ConfigDir, name)
	wrote, err := startup.WriteFile(filepath.Join(homeDir, rel), content, 0o644)
	if err != nil || !wrote {
		return nil, err
	}
	return []Change{{rel, "written"}}, nil
}

// hookSpec is one marked line to keep in a file Hiwthi doesn't own.
type hookSpec struct {
	rel   string // relative to the home
	line  startup.Line
	extra []string // written after the line when the file is created
}

// hook makes sure the file has Hiwthi's line.
func hook(homeDir string, h hookSpec) ([]Change, error) {
	path := filepath.Join(homeDir, h.rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	r, err := startup.Ensure(path, h.line, h.extra...)
	if err != nil || r == startup.Unchanged {
		return nil, err
	}
	return []Change{{h.rel, fmt.Sprintf("line %s", r)}}, nil
}

// inHome returns a function telling whether a file, relative to homeDir,
// exists (a dangling symbolic link included).
func inHome(homeDir string) func(string) bool {
	return func(rel string) bool {
		_, err := os.Lstat(filepath.Join(homeDir, rel))
		return err == nil
	}
}
