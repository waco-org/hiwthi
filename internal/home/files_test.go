package home

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/waco-org/hiwthi/internal/config"
	"github.com/waco-org/hiwthi/internal/startup"
)

const backup = ".local/state/hiwthi/backup/test"

// repoWith creates a repo whose files/ holds the given files (name →
// content); a name ending in "*" is made executable.
func repoWith(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	for name, content := range files {
		mode := fs.FileMode(0o644)
		if strings.HasSuffix(name, "*") {
			name, mode = strings.TrimSuffix(name, "*"), 0o755
		}
		path := filepath.Join(repo, FilesDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

// encrypt writes files/<name>.age in repo, encrypted to recipient.
func encrypt(t *testing.T, repo, name, content string, recipient age.Recipient) {
	t.Helper()
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, FilesDir, name+".age")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// giveKey puts id in the home's key file.
func giveKey(t *testing.T, home string, id *age.X25519Identity) {
	t.Helper()
	path := filepath.Join(home, KeyFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newKey(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func applyFiles(t *testing.T, home, repo string, c *config.Config) map[string]string {
	t.Helper()
	changes, err := ApplyFiles(home, repo, backup, c)
	if err != nil {
		t.Fatalf("ApplyFiles: %v", err)
	}
	m := map[string]string{}
	for _, ch := range changes {
		m[ch.Path] = ch.What
	}
	return m
}

func mode(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestNoFilesDir(t *testing.T) {
	if got := applyFiles(t, t.TempDir(), t.TempDir(), &config.Config{}); len(got) != 0 {
		t.Errorf("changes = %v", got)
	}
}

func TestCopiesFiles(t *testing.T) {
	home := t.TempDir()
	repo := repoWith(t, map[string]string{
		".vimrc":              "set nu\n",
		"bin/tool*":           "#!/bin/sh\n",
		".ssh/config":         "Host *\n",
		".config/app/app.ini": "x=1\n",
	})
	got := applyFiles(t, home, repo, &config.Config{})
	if len(got) != 4 {
		t.Errorf("changes = %v", got)
	}
	if read(t, home, ".vimrc") != "set nu\n" {
		t.Error(".vimrc not copied")
	}
	for path, want := range map[string]fs.FileMode{
		".vimrc": 0o644, "bin/tool": 0o755, ".ssh": 0o700, ".ssh/config": 0o644, ".config/app": 0o755,
	} {
		if m := mode(t, filepath.Join(home, path)); m != want {
			t.Errorf("%s: mode %v, want %v", path, m, want)
		}
	}
	if again := applyFiles(t, home, repo, &config.Config{}); len(again) != 0 {
		t.Errorf("second apply changed %v", again)
	}
}

func TestReplacesAndSavesOldCopy(t *testing.T) {
	home := t.TempDir()
	write(t, home, ".vimrc", "mine\n")
	got := applyFiles(t, home, repoWith(t, map[string]string{".vimrc": "repo\n"}), &config.Config{})
	if !strings.HasPrefix(got[".vimrc"], "replaced") {
		t.Errorf("changes = %v", got)
	}
	if read(t, home, ".vimrc") != "repo\n" || read(t, home, backup+"/.vimrc") != "mine\n" {
		t.Error("expected the repo's copy in place and the old one saved")
	}
}

func TestDecryptsWithKey(t *testing.T) {
	home := t.TempDir()
	id := newKey(t)
	giveKey(t, home, id)
	repo := repoWith(t, nil)
	encrypt(t, repo, ".pypirc", "[pypi]\npassword = s3cret\n", id.Recipient())
	got := applyFiles(t, home, repo, &config.Config{})
	if got[".pypirc"] != "created" {
		t.Errorf("changes = %v", got)
	}
	if read(t, home, ".pypirc") != "[pypi]\npassword = s3cret\n" {
		t.Error("not decrypted")
	}
	if m := mode(t, filepath.Join(home, ".pypirc")); m != 0o600 {
		t.Errorf("mode %v, want 0600", m)
	}
}

func TestSkipsEncryptedWithoutKey(t *testing.T) {
	home := t.TempDir()
	repo := repoWith(t, map[string]string{".vimrc": "x\n"})
	encrypt(t, repo, ".pypirc", "secret", newKey(t).Recipient())
	got := applyFiles(t, home, repo, &config.Config{})
	if len(got) != 1 || got[".vimrc"] != "created" {
		t.Errorf("changes = %v, want only .vimrc", got)
	}
}

func TestWrongKeyIsAnError(t *testing.T) {
	home := t.TempDir()
	giveKey(t, home, newKey(t))
	repo := repoWith(t, map[string]string{".vimrc": "x\n"})
	encrypt(t, repo, ".pypirc", "secret", newKey(t).Recipient())
	changes, err := ApplyFiles(home, repo, backup, &config.Config{})
	if err == nil || !strings.Contains(err.Error(), ".pypirc.age") {
		t.Errorf("err = %v, want one about .pypirc.age", err)
	}
	if len(changes) != 1 {
		t.Errorf("changes = %v, want .vimrc copied anyway", changes)
	}
}

func TestPlainAndEncryptedClash(t *testing.T) {
	home := t.TempDir()
	id := newKey(t)
	giveKey(t, home, id)
	repo := repoWith(t, map[string]string{".pypirc": "plain"})
	encrypt(t, repo, ".pypirc", "secret", id.Recipient())
	if _, err := ApplyFiles(home, repo, backup, &config.Config{}); err == nil {
		t.Error("expected an error")
	}
}

func TestRefusesOwnedFiles(t *testing.T) {
	repo := repoWith(t, map[string]string{
		".config/hiwthi/env.sh":    "x",
		".config/hiwthi/local.zsh": "setopt COMPLETE_IN_WORD\n",
	})
	home := t.TempDir()
	changes, err := ApplyFiles(home, repo, backup, &config.Config{})
	if err == nil || !strings.Contains(err.Error(), "env.sh") {
		t.Errorf("err = %v, want one about env.sh", err)
	}
	if len(changes) != 1 || changes[0].Path != ".config/hiwthi/local.zsh" {
		t.Errorf("changes = %v, want local.zsh", changes)
	}
}

// A startup file in files/ gets Hiwthi's line as it is copied, so files/ and
// the shell part don't undo each other.
func TestStartupFileFromRepoConverges(t *testing.T) {
	home := t.TempDir()
	repo := repoWith(t, map[string]string{".zshrc": "setopt AUTO_CD\n", ".profile": "# mine\n"})
	c := &config.Config{Shell: &config.Shell{Shells: []string{"bash", "zsh"}}}
	applyFiles(t, home, repo, c)
	apply(t, home, c)
	zshrc := read(t, home, ".zshrc")
	if !strings.HasPrefix(zshrc, "setopt AUTO_CD\n") ||
		!strings.Contains(zshrc, startup.SourceLine("interactive.sh").Text) {
		t.Errorf(".zshrc = %q", zshrc)
	}
	// .profile from files/ is bash's login file: no .bash_profile appears.
	if _, err := os.Stat(filepath.Join(home, ".bash_profile")); err == nil {
		t.Error(".bash_profile created next to the repo's .profile")
	}
	if again := applyFiles(t, home, repo, c); len(again) != 0 {
		t.Errorf("files changed again: %v", again)
	}
	if again := apply(t, home, c); len(again) != 0 {
		t.Errorf("shell changed again: %v", again)
	}
}

func TestWritesThroughSymlinkInHome(t *testing.T) {
	home := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "vimrc")
	if err := os.WriteFile(elsewhere, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(home, ".vimrc")); err != nil {
		t.Fatal(err)
	}
	applyFiles(t, home, repoWith(t, map[string]string{".vimrc": "new\n"}), &config.Config{})
	if fi, _ := os.Lstat(filepath.Join(home, ".vimrc")); fi.Mode()&fs.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	if got, _ := os.ReadFile(elsewhere); string(got) != "new\n" {
		t.Errorf("link target = %q", got)
	}
}

func TestCopiesSymlinks(t *testing.T) {
	repo := repoWith(t, nil)
	if err := os.MkdirAll(filepath.Join(repo, FilesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".vimrc", filepath.Join(repo, FilesDir, ".gvimrc")); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	applyFiles(t, home, repo, &config.Config{})
	if target, err := os.Readlink(filepath.Join(home, ".gvimrc")); err != nil || target != ".vimrc" {
		t.Errorf("link = %q, %v", target, err)
	}
	if again := applyFiles(t, home, repo, &config.Config{}); len(again) != 0 {
		t.Errorf("second apply changed %v", again)
	}
}

func TestDirectoryInTheWay(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".vimrc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyFiles(home, repoWith(t, map[string]string{".vimrc": "x"}), backup,
		&config.Config{}); err == nil {
		t.Error("expected an error")
	}
}
