// Package repo handles the user's Hiwthi repo: the git clone holding
// common.yml, local.yml and files/. Git is built in (go-git), so a machine
// needs no git of its own.
package repo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
)

// go-git serves local paths and file:// URLs by running the system's git;
// its own server does the same in process, so no git is needed at all. Its
// default loader only opens bare repositories, hence workTreeLoader.
func init() {
	client.InstallProtocol("file", server.NewServer(workTreeLoader{}))
}

// workTreeLoader opens a repository for go-git's server: the .git directory
// of a working copy, or the path itself for a bare repository.
type workTreeLoader struct{}

func (workTreeLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	path := ep.Path
	if fi, err := os.Stat(filepath.Join(path, git.GitDirName)); err == nil && fi.IsDir() {
		path = filepath.Join(path, git.GitDirName)
	}
	return server.DefaultLoader.Load(&transport.Endpoint{Protocol: ep.Protocol, Path: path})
}

// ErrAlreadyCloned is returned by Clone when the directory already holds a
// clone of the same URL: init has been run before.
var ErrAlreadyCloned = errors.New("already cloned")

// CloneOptions say what to clone where.
type CloneOptions struct {
	URL    string
	Dir    string
	Branch string    // the variant; empty for the remote's default branch
	Output io.Writer // git's progress messages; nil for none
}

// Clone clones the user's repo into o.Dir. It clones into a temporary
// directory next to it first, so a failed clone leaves nothing behind.
func Clone(o CloneOptions) error {
	if err := checkTarget(o.Dir, o.URL); err != nil {
		return err
	}
	parent := filepath.Dir(o.Dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(o.Dir)+".init-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) // empty after the rename

	opts := &git.CloneOptions{URL: o.URL, Progress: o.Output}
	if o.Branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(o.Branch)
		opts.SingleBranch = false
	}
	if _, err := git.PlainClone(tmp, false, opts); err != nil {
		return fmt.Errorf("cloning %s: %w%s", o.URL, err, hint(o.URL, err))
	}
	return os.Rename(tmp, o.Dir)
}

// checkTarget makes sure dir is free for the clone: missing or empty. A clone
// of the same URL gives ErrAlreadyCloned.
func checkTarget(dir, url string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(dir) // the rename needs it gone
	}
	if r, err := git.PlainOpen(dir); err == nil {
		if origin, err := r.Remote("origin"); err == nil &&
			len(origin.Config().URLs) > 0 && origin.Config().URLs[0] == url {
			return fmt.Errorf("%s: %w from %s", dir, ErrAlreadyCloned, url)
		}
	}
	return fmt.Errorf("%s already exists and isn't empty", dir)
}

// hint adds advice for the errors people meet first with SSH URLs.
func hint(url string, err error) string {
	ssh := strings.HasPrefix(url, "ssh://") ||
		(strings.Contains(url, "@") && !strings.Contains(url, "://"))
	if !ssh {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "SSH_AUTH_SOCK"):
		return "\n(Hiwthi uses your SSH agent: start one and add your key with ssh-add)"
	case strings.Contains(msg, "knownhosts"):
		return "\n(the server isn't in ~/.ssh/known_hosts yet: connect once with ssh to check and add it)"
	case strings.Contains(msg, "unable to authenticate"):
		return "\n(is your key in the agent (ssh-add -l) and registered with the server?)"
	}
	return ""
}
