package repo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// source creates a repo with common.yml on main and local.yml added on
// branch "laptop", and returns its path.
func source(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	commit := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatal(err)
		}
		sig := &object.Signature{Name: "Test", Email: "test@example.org", When: time.Now()}
		if _, err := wt.Commit("add "+name, &git.CommitOptions{Author: sig}); err != nil {
			t.Fatal(err)
		}
	}
	commit("common.yml", "hiwthi: 1\n")
	if err := wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("laptop"), Create: true,
	}); err != nil {
		t.Fatal(err)
	}
	commit("local.yml", "shell:\n  editing_mode: vi\n")
	if err := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("main")}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestCloneDefaultBranch(t *testing.T) {
	src := source(t)
	dir := filepath.Join(t.TempDir(), "share", "hiwthi", "repo")
	if err := Clone(CloneOptions{URL: src, Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, "common.yml")) || exists(filepath.Join(dir, "local.yml")) {
		t.Error("expected main: common.yml without local.yml")
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(dir), ".repo.init-*"))
	if len(leftovers) > 0 {
		t.Errorf("temporary directories left: %v", leftovers)
	}
}

func TestCloneBranch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "repo")
	if err := Clone(CloneOptions{URL: source(t), Dir: dir, Branch: "laptop"}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, "local.yml")) {
		t.Error("local.yml missing: branch laptop not checked out")
	}
}

func TestCloneIntoEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := Clone(CloneOptions{URL: source(t), Dir: dir}); err != nil {
		t.Fatal(err)
	}
}

func TestCloneTwice(t *testing.T) {
	src := source(t)
	dir := filepath.Join(t.TempDir(), "repo")
	if err := Clone(CloneOptions{URL: src, Dir: dir}); err != nil {
		t.Fatal(err)
	}
	err := Clone(CloneOptions{URL: src, Dir: dir})
	if !errors.Is(err, ErrAlreadyCloned) {
		t.Errorf("err = %v, want ErrAlreadyCloned", err)
	}
	err = Clone(CloneOptions{URL: source(t), Dir: dir})
	if err == nil || errors.Is(err, ErrAlreadyCloned) {
		t.Errorf("another URL: err = %v, want a plain error", err)
	}
}

func TestCloneRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Clone(CloneOptions{URL: source(t), Dir: dir}); err == nil {
		t.Error("expected an error")
	}
}

func TestFailedCloneLeavesNothing(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	err := Clone(CloneOptions{URL: filepath.Join(parent, "no-such-repo"), Dir: dir})
	if err == nil {
		t.Fatal("expected an error")
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}

func TestHint(t *testing.T) {
	err := errors.New("ssh: handshake failed: knownhosts: key is unknown")
	if h := hint("git@gitlab.com:me/home.git", err); !strings.Contains(h, "known_hosts") {
		t.Errorf("hint = %q", h)
	}
	if h := hint("https://github.com/me/home.git", err); h != "" {
		t.Errorf("hint for https = %q", h)
	}
}

// commitIn adds a file to the repo at dir on its current branch.
func commitIn(t *testing.T, dir, name, content string) {
	t.Helper()
	r, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "Test", Email: "test@example.org", When: time.Now()}
	if _, err := wt.Commit("add "+name, &git.CommitOptions{Author: sig}); err != nil {
		t.Fatal(err)
	}
}

func checkout(t *testing.T, dir, branch string) {
	t.Helper()
	r, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(branch)}); err != nil {
		t.Fatal(err)
	}
}

func cloned(t *testing.T, src, branch string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := Clone(CloneOptions{URL: src, Dir: dir, Branch: branch}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPullUpToDate(t *testing.T) {
	dir := cloned(t, source(t), "")
	if changed, err := Pull(dir, nil); err != nil || changed {
		t.Errorf("Pull = %v, %v; want false, nil", changed, err)
	}
}

// The branch checked out is pulled, not the remote's default one.
func TestPullFollowsBranch(t *testing.T) {
	src := source(t)
	dir := cloned(t, src, "laptop")
	commitIn(t, src, "on-main", "x") // src has main checked out
	checkout(t, src, "laptop")
	commitIn(t, src, "on-laptop", "x")
	checkout(t, src, "main")

	if changed, err := Pull(dir, nil); err != nil || !changed {
		t.Fatalf("Pull = %v, %v; want true, nil", changed, err)
	}
	if !exists(filepath.Join(dir, "on-laptop")) {
		t.Error("laptop's new commit missing")
	}
	if exists(filepath.Join(dir, "on-main")) {
		t.Error("main's commit pulled into laptop")
	}
}

func TestPullRefusesUncommittedChanges(t *testing.T) {
	src := source(t)
	dir := cloned(t, src, "")
	commitIn(t, src, "new", "x")
	if err := os.WriteFile(filepath.Join(dir, "common.yml"), []byte("hiwthi: 1\n# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(dir, nil); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("err = %v, want uncommitted changes", err)
	}
}

func TestPullRefusesDivergence(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("go-git's in-process server, used without git, reports divergence as \"object not found\"")
	}
	src := source(t)
	dir := cloned(t, src, "")
	commitIn(t, src, "theirs", "x")
	commitIn(t, dir, "mine", "y")
	if _, err := Pull(dir, nil); err == nil || !strings.Contains(err.Error(), "push or merge") {
		t.Errorf("err = %v, want a divergence error", err)
	}
}
