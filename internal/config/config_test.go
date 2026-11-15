package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeRepo creates a repo directory with the given files and returns it.
// An empty local means no local.yml at all.
func writeRepo(t *testing.T, common, local string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(CommonFile, common)
	if local != "" {
		write(LocalFile, local)
	}
	return dir
}

func load(t *testing.T, common, local string) *Config {
	t.Helper()
	c, err := Load(writeRepo(t, common, local))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func loadError(t *testing.T, common, local string) string {
	t.Helper()
	_, err := Load(writeRepo(t, common, local))
	if err == nil {
		t.Fatal("Load succeeded, expected an error")
	}
	return err.Error()
}

func TestScalarInLocalReplacesCommon(t *testing.T) {
	c := load(t, "hiwthi: 1\nshell:\n  editing_mode: emacs\n",
		"shell:\n  editing_mode: vi\n")
	if c.Shell.EditingMode != "vi" {
		t.Errorf("editing_mode = %q, want vi", c.Shell.EditingMode)
	}
}

func TestListsAreJoinedWithoutDuplicates(t *testing.T) {
	c := load(t, "hiwthi: 1\nflatpaks: [a, b]\n", "flatpaks: [b, c]\n")
	if want := []string{"a", "b", "c"}; !slices.Equal(c.Flatpaks, want) {
		t.Errorf("flatpaks = %v, want %v", c.Flatpaks, want)
	}
}

func TestNestedListsAreComparedByContent(t *testing.T) {
	common := `hiwthi: 1
gnome:
  settings:
    org.gnome.desktop.input-sources:
      sources: [[xkb, us], [xkb, it]]
`
	local := `gnome:
  settings:
    org.gnome.desktop.input-sources:
      sources: [[xkb, it], [xkb, de]]
`
	n, err := LoadNode(writeRepo(t, common, local))
	if err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := n.Decode(&c); err != nil {
		t.Fatal(err)
	}
	node := c.Gnome.Settings["org.gnome.desktop.input-sources"]["sources"]
	var sources [][]string
	if err := node.Decode(&sources); err != nil {
		t.Fatal(err)
	}
	if len(sources) != 3 || sources[2][1] != "de" {
		t.Errorf("sources = %v, want us, it, de", sources)
	}
}

func TestDictionariesAreMergedKeyByKey(t *testing.T) {
	common := `hiwthi: 1
gnome:
  settings:
    org.gnome.shell.extensions.dash-to-dock:
      dock-position: BOTTOM
`
	local := `gnome:
  settings:
    org.gnome.shell.extensions.dash-to-dock:
      preferred-monitor-by-connector: HDMI-1
`
	c := load(t, common, local)
	dock := c.Gnome.Settings["org.gnome.shell.extensions.dash-to-dock"]
	if dock["dock-position"].Value != "BOTTOM" ||
		dock["preferred-monitor-by-connector"].Value != "HDMI-1" {
		t.Errorf("dash-to-dock settings = %v, want both keys", dock)
	}
}

func TestEmptyKeyInLocalLeavesCommonAlone(t *testing.T) {
	c := load(t, "hiwthi: 1\nflatpaks: [a]\n", "flatpaks:\n")
	if !slices.Equal(c.Flatpaks, []string{"a"}) {
		t.Errorf("flatpaks = %v, want [a]", c.Flatpaks)
	}
}

func TestLocalIsOptionalAndMayHoldOnlyComments(t *testing.T) {
	load(t, "hiwthi: 1\n", "")
	load(t, "hiwthi: 1\n", "# nothing yet\n")
}

func TestPythonVersionsKeepTheirText(t *testing.T) {
	c := load(t, "hiwthi: 1\npython:\n  versions: [3.10, \"3.11\"]\n", "")
	if want := []string{"3.10", "3.11"}; !slices.Equal(c.Python.Versions, want) {
		t.Errorf("versions = %v, want %v", c.Python.Versions, want)
	}
}

// Where Config fixes the type, each file's own check catches a mismatch,
// with its line.
func TestWrongTypeInOneFile(t *testing.T) {
	msg := loadError(t, "hiwthi: 1\nflatpaks: [a]\n", "flatpaks: a\n")
	if !strings.Contains(msg, LocalFile) || !strings.Contains(msg, "line 1") {
		t.Errorf("error = %q", msg)
	}
}

// GNOME setting values have no fixed type, so only the merge sees that the
// two files disagree.
func TestDifferentKindsAreAnError(t *testing.T) {
	common := "hiwthi: 1\ngnome:\n  settings:\n    s:\n      k: [a]\n"
	local := "gnome:\n  settings:\n    s:\n      k: a\n"
	msg := loadError(t, common, local)
	if !strings.Contains(msg, "gnome.settings.s.k: common.yml has a list, local.yml a single value") {
		t.Errorf("error = %q", msg)
	}
}

func TestUnknownKeyNamesFileAndLine(t *testing.T) {
	msg := loadError(t, "hiwthi: 1\n", "shell:\n  editng_mode: vi\n")
	if !strings.Contains(msg, LocalFile) || !strings.Contains(msg, "line 2") ||
		!strings.Contains(msg, "editng_mode") {
		t.Errorf("error = %q, want file, line and key", msg)
	}
}

func TestFormatVersion(t *testing.T) {
	if msg := loadError(t, "flatpaks: []\n", ""); !strings.Contains(msg, "missing") {
		t.Errorf("no version: error = %q", msg)
	}
	if msg := loadError(t, "hiwthi: 2\n", ""); !strings.Contains(msg, "update Hiwthi") {
		t.Errorf("newer version: error = %q", msg)
	}
}

func TestShellValuesAreChecked(t *testing.T) {
	loadError(t, "hiwthi: 1\nshell:\n  shells: [fish]\n", "")
	loadError(t, "hiwthi: 1\nshell:\n  editing_mode: ed\n", "")
}

func TestMissingCommonIsAnError(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Error("Load succeeded without common.yml")
	}
}

// The owner's draft from the POC (hiwthi-conversation drafts/config) must
// load, with imladris's local.yml merged in.
func TestImladrisDraft(t *testing.T) {
	c, err := Load(filepath.Join("testdata", "imladris"))
	if err != nil {
		t.Fatal(err)
	}
	dock := c.Gnome.Settings["org.gnome.shell.extensions.dash-to-dock"]
	if dock["preferred-monitor-by-connector"].Value != "HDMI-1" || dock["dock-position"].Value != "BOTTOM" {
		t.Errorf("dash-to-dock = %v", dock)
	}
	if !slices.Contains(c.Python.Versions, "3.10") || c.Tools["rust"] != "stable" {
		t.Errorf("python = %v, tools = %v", c.Python, c.Tools)
	}
}
