package startup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var envLine = SourceLine("env.sh")

func ensure(t *testing.T, before string, extra ...string) (Result, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".zshenv")
	if before != "" {
		if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Ensure(path, envLine, extra...)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return r, string(after)
}

func TestCreatesMissingFile(t *testing.T) {
	r, got := ensure(t, "", "extra")
	want := Comment + "\n" + envLine.Text + "\nextra\n"
	if r != Created || got != want {
		t.Errorf("got %v %q, want created %q", r, got, want)
	}
}

func TestAppendsAfterExistingContent(t *testing.T) {
	r, got := ensure(t, "export A=1") // no final newline
	want := "export A=1\n\n" + Comment + "\n" + envLine.Text + "\n"
	if r != Added || got != want {
		t.Errorf("got %v %q, want added %q", r, got, want)
	}
}

func TestUnchangedWhenPresent(t *testing.T) {
	before := "a\n" + envLine.Text + "\nb\n"
	if r, got := ensure(t, before); r != Unchanged || got != before {
		t.Errorf("got %v %q, want unchanged", r, got)
	}
}

func TestReplacesInPlace(t *testing.T) {
	r, got := ensure(t, "a\nsource $HOME/.config/hiwthi/env.sh\r\nb\n")
	want := "a\n" + envLine.Text + "\r\nb\n"
	if r != Replaced || got != want {
		t.Errorf("got %v %q, want replaced %q", r, got, want)
	}
}

func TestCommentsDontCount(t *testing.T) {
	before := "# . ~/.config/hiwthi/env.sh\n"
	r, got := ensure(t, before)
	if r != Added || got != before+"\n"+Comment+"\n"+envLine.Text+"\n" {
		t.Errorf("got %v %q", r, got)
	}
}

func TestOtherTargetsDontCount(t *testing.T) {
	before := SourceLine("interactive.sh").Text + "\n"
	if r, _ := ensure(t, before); r != Added {
		t.Errorf("got %v, want added", r)
	}
}

func TestSeveralLinesLeaveFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshenv")
	before := ". ~/.config/hiwthi/env.sh\nx\nsource ~/.config/hiwthi/env.sh\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Ensure(path, envLine)
	if !errors.Is(err, ErrSeveral) {
		t.Fatalf("err = %v, want ErrSeveral", err)
	}
	if after, _ := os.ReadFile(path); string(after) != before {
		t.Errorf("file changed: %q", after)
	}
}

func TestKeepsPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshenv")
	if err := os.WriteFile(path, []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(path, envLine); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles-zshenv")
	link := filepath.Join(dir, ".zshenv")
	if err := os.WriteFile(target, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(link, envLine); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link replaced by a file (err %v)", err)
	}
	if got, _ := os.ReadFile(target); string(got) == "a\n" {
		t.Error("target unchanged")
	}
}

func TestWriteFileOnlyWhenDifferent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "f")
	for i, want := range []bool{true, false} {
		wrote, err := WriteFile(path, "x\n", 0o644)
		if err != nil || wrote != want {
			t.Errorf("run %d: wrote = %v, err = %v; want %v", i+1, wrote, err, want)
		}
	}
}
