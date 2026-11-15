// Package config reads a home's description: common.yml and local.yml in the
// user's Hiwthi repo, merged into one Config.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"
)

// FormatVersion is the version of the file format this Hiwthi reads: the
// value of the hiwthi key at the top of common.yml.
const FormatVersion = 1

// File names in the user's repo.
const (
	CommonFile = "common.yml"
	LocalFile  = "local.yml"
)

// Config is the merged description of a home. Pointers and nil maps or
// slices stand for sections that are absent: Hiwthi leaves alone what they
// would manage.
type Config struct {
	Version  int               `yaml:"hiwthi"`
	Identity *Identity         `yaml:"identity,omitempty"`
	Shell    *Shell            `yaml:"shell,omitempty"`
	Tools    map[string]string `yaml:"tools,omitempty"`
	Python   *Python           `yaml:"python,omitempty"`
	Flatpaks []string          `yaml:"flatpaks,omitempty"`
	Gnome    *Gnome            `yaml:"gnome,omitempty"`
}

// Identity is written into git's and Mercurial's configuration.
type Identity struct {
	Name  string `yaml:"name,omitempty"`
	Email string `yaml:"email,omitempty"`
}

// Shell describes the files Hiwthi owns in ~/.config/hiwthi (env.sh,
// interactive.sh) and the shells that source them.
type Shell struct {
	Shells      []string          `yaml:"shells,omitempty"`
	EditingMode string            `yaml:"editing_mode,omitempty"`
	Path        []string          `yaml:"path,omitempty"`
	PathAppend  []string          `yaml:"path_append,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Aliases     map[string]string `yaml:"aliases,omitempty"`
}

// Python lists what uv installs.
type Python struct {
	Versions []string `yaml:"versions,omitempty"`
	Tools    []string `yaml:"tools,omitempty"`
}

// Gnome holds the user's GNOME settings.
type Gnome struct {
	Extensions  []string            `yaml:"extensions,omitempty"`
	Favorites   []string            `yaml:"favorites,omitempty"`
	Autostart   []string            `yaml:"autostart,omitempty"`
	DefaultApps map[string][]string `yaml:"default_apps,omitempty"`
	// Values stay YAML nodes: their GSettings type comes from the schema
	// installed on the machine, which only the code applying them knows.
	Settings map[string]map[string]yaml.Node `yaml:"settings,omitempty"`
}

// Supported values of Shell.Shells and Shell.EditingMode.
var (
	supportedShells = []string{"bash", "zsh"}
	editingModes    = []string{"emacs", "vi"}
)

// Load reads common.yml and, if present, local.yml from dir, checks each,
// merges them and returns the result.
func Load(dir string) (*Config, error) {
	merged, err := LoadNode(dir)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := merged.Decode(&c); err != nil {
		return nil, fmt.Errorf("merging %s and %s: %w", CommonFile, LocalFile, err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// LoadNode is Load without decoding: the merged document as a YAML node, to
// show it as Hiwthi sees it.
func LoadNode(dir string) (*yaml.Node, error) {
	common, err := readFile(filepath.Join(dir, CommonFile), true)
	if err != nil {
		return nil, err
	}
	local, err := readFile(filepath.Join(dir, LocalFile), false)
	if err != nil {
		return nil, err
	}
	return merge(common, local, "")
}

// readFile parses one file and checks it against Config, so that errors such
// as a misspelt key point at the right file and line. It returns the file's
// top-level mapping, or nil for a missing optional file or an empty one.
func readFile(path string, required bool) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && !required {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var check Config
	if err := dec.Decode(&check); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return nil, nil // only comments
	}
	return doc.Content[0], nil
}

func (c *Config) validate() error {
	switch c.Version {
	case FormatVersion:
	case 0:
		return fmt.Errorf("%s: missing \"hiwthi: %d\" (the format version) at the top",
			CommonFile, FormatVersion)
	default:
		return fmt.Errorf("format version %d, this Hiwthi reads version %d: update Hiwthi",
			c.Version, FormatVersion)
	}
	if c.Shell != nil {
		for _, s := range c.Shell.Shells {
			if !slices.Contains(supportedShells, s) {
				return fmt.Errorf("shell.shells: %q isn't supported (supported: %v)",
					s, supportedShells)
			}
		}
		if m := c.Shell.EditingMode; m != "" && !slices.Contains(editingModes, m) {
			return fmt.Errorf("shell.editing_mode: %q, expected one of %v", m, editingModes)
		}
	}
	return nil
}
