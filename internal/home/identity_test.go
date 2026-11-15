package home

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/waco-org/hiwthi/internal/config"
)

// withTools makes lookPath find only the given commands.
func withTools(t *testing.T, tools ...string) {
	t.Helper()
	old := lookPath
	lookPath = func(name string) (string, error) {
		if slices.Contains(tools, name) {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPath = old })
}

func identity(name, email string) *config.Config {
	return &config.Config{Identity: &config.Identity{Name: name, Email: email}}
}

func applyIdentity(t *testing.T, home string, c *config.Config) map[string]string {
	t.Helper()
	changes, err := ApplyIdentity(home, c)
	if err != nil {
		t.Fatalf("ApplyIdentity: %v", err)
	}
	m := map[string]string{}
	for _, ch := range changes {
		m[ch.Path] = ch.What
	}
	return m
}

func TestIdentityForBothTools(t *testing.T) {
	withTools(t, "git", "hg")
	home := t.TempDir()
	got := applyIdentity(t, home, identity("Ann O'Neil", "ann@example.org"))
	want := map[string]string{
		".config/hiwthi/gitconfig": "written",
		".config/hiwthi/hgrc":      "written",
		".gitconfig":               "line created",
		".hgrc":                    "line created",
	}
	if len(got) != len(want) {
		t.Errorf("changes = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
	if hg := read(t, home, ".config/hiwthi/hgrc"); !strings.Contains(hg,
		"\n[ui]\nusername = Ann O'Neil <ann@example.org>\n") {
		t.Errorf("hgrc:\n%s", hg)
	}
	if inc := read(t, home, ".hgrc"); !strings.Contains(inc, "\n%include ~/.config/hiwthi/hgrc\n") {
		t.Errorf(".hgrc:\n%s", inc)
	}
	if again := applyIdentity(t, home, identity("Ann O'Neil", "ann@example.org")); len(again) != 0 {
		t.Errorf("second apply changed %v", again)
	}
}

func TestIdentitySkipsMissingTools(t *testing.T) {
	withTools(t)
	home := t.TempDir()
	if got := applyIdentity(t, home, identity("Ann", "ann@example.org")); len(got) != 0 {
		t.Errorf("changes = %v, want none", got)
	}
}

func TestNoIdentityDoesNothing(t *testing.T) {
	withTools(t, "git", "hg")
	for _, c := range []*config.Config{{}, identity("", "")} {
		if got := applyIdentity(t, t.TempDir(), c); len(got) != 0 {
			t.Errorf("changes = %v, want none", got)
		}
	}
}

func TestIdentityUsesXdgFileIfOnlyOne(t *testing.T) {
	withTools(t, "git")
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config/git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, home, ".config/git/config", "[core]\n\teditor = vim\n")
	got := applyIdentity(t, home, identity("Ann", ""))
	if got[".config/git/config"] != "line added" {
		t.Errorf("changes = %v", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".gitconfig")); !errors.Is(err, os.ErrNotExist) {
		t.Error(".gitconfig created next to .config/git/config")
	}
}

func TestIdentityRejectsNewlines(t *testing.T) {
	withTools(t, "git")
	if _, err := ApplyIdentity(t.TempDir(), identity("Ann\nB", "")); err == nil {
		t.Error("expected an error")
	}
}

// TestGitReadsIdentity checks with the real git that the include works and
// that the quoting survives awkward values.
func TestGitReadsIdentity(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	withTools(t, "git")
	home := t.TempDir()
	// The user's own setting earlier in the file loses to Hiwthi's include,
	// appended after it.
	write(t, home, ".gitconfig", "[user]\n\tname = Old Name\n")
	name, email := `Ann "Nan" O'Neil \ #1;`, "ann@example.org"
	applyIdentity(t, home, identity(name, email))
	for key, want := range map[string]string{"user.name": name, "user.email": email} {
		// Not --global, which ignores includes unless asked: the way git
		// itself reads its configuration, outside a repository.
		cmd := exec.Command(gitBin, "config", key)
		cmd.Dir = home
		cmd.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CEILING_DIRECTORIES=" + filepath.Dir(home)}
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git config %s: %v", key, err)
		}
		if got := strings.TrimSuffix(string(out), "\n"); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
