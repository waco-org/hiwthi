package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waco-org/hiwthi/internal/config"
	"github.com/waco-org/hiwthi/internal/startup"
)

func write(t *testing.T, home, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, home, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func apply(t *testing.T, home string, c *config.Config) map[string]string {
	t.Helper()
	changes, err := ApplyShell(home, c)
	if err != nil {
		t.Fatalf("ApplyShell: %v", err)
	}
	m := map[string]string{}
	for _, ch := range changes {
		m[ch.Path] = ch.What
	}
	return m
}

var bashAndZsh = &config.Config{Shell: &config.Shell{Shells: []string{"bash", "zsh"}}}

func TestNoShellSectionDoesNothing(t *testing.T) {
	home := t.TempDir()
	if got := apply(t, home, &config.Config{}); len(got) != 0 {
		t.Errorf("changes = %v, want none", got)
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Errorf("home not empty: %v", entries)
	}
}

func TestFedoraLikeHome(t *testing.T) {
	home := t.TempDir()
	write(t, home, ".bash_profile", "[ -f ~/.bashrc ] && . ~/.bashrc\n")
	write(t, home, ".bashrc", "# distro\n")
	got := apply(t, home, bashAndZsh)
	want := map[string]string{
		".config/hiwthi/env.sh":         "written",
		".config/hiwthi/interactive.sh": "written",
		".bash_profile":                 "line added",
		".bashrc":                       "line added",
		".zshenv":                       "line created",
		".zshrc":                        "line created",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("changes = %v", got)
	}
	if again := apply(t, home, bashAndZsh); len(again) != 0 {
		t.Errorf("second apply changed %v", again)
	}
}

func TestUbuntuLikeHomeKeepsProfile(t *testing.T) {
	home := t.TempDir()
	write(t, home, ".profile", "# ubuntu\n")
	got := apply(t, home, bashAndZsh)
	if got[".profile"] != "line added" {
		t.Errorf(".profile: %q, want line added", got[".profile"])
	}
	if _, err := os.Stat(filepath.Join(home, ".bash_profile")); err == nil {
		t.Error(".bash_profile created: it would hide .profile")
	}
}

func TestBareHomeGetsBashProfileReadingBashrc(t *testing.T) {
	home := t.TempDir()
	apply(t, home, &config.Config{Shell: &config.Shell{Shells: []string{"bash"}}})
	got := read(t, home, ".bash_profile")
	want := startup.Comment + "\n" + startup.SourceLine("env.sh").Text + "\n" + bashrcFromProfile + "\n"
	if got != want {
		t.Errorf(".bash_profile = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); err == nil {
		t.Error(".zshrc created for a description naming only bash")
	}
}

func TestOneBadFileDoesntStopTheOthers(t *testing.T) {
	home := t.TempDir()
	write(t, home, ".zshrc", ". ~/.config/hiwthi/interactive.sh\n. ~/.config/hiwthi/interactive.sh\n")
	_, err := ApplyShell(home, bashAndZsh)
	if err == nil || !strings.Contains(err.Error(), ".zshrc") {
		t.Fatalf("err = %v, want one about .zshrc", err)
	}
	if !strings.Contains(read(t, home, ".zshenv"), "env.sh") {
		t.Error(".zshenv not hooked")
	}
}
