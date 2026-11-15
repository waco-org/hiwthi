// Package home applies a description to a home directory, one part at a
// time.
package home

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/waco-org/hiwthi/internal/config"
	"github.com/waco-org/hiwthi/internal/shell"
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

// bashLoginFiles are the files a bash login shell reads, in bash's order: it
// reads only the first that exists. Hiwthi hooks into that one, so on
// Ubuntu, where only .profile exists, it doesn't create a .bash_profile that
// would hide it.
var bashLoginFiles = []string{".bash_profile", ".bash_login", ".profile"}

// bashrcFromProfile is written in a .bash_profile Hiwthi creates: bash login
// shells don't read .bashrc by themselves.
const bashrcFromProfile = "[ -r ~/.bashrc ] && . ~/.bashrc"

// ApplyShell writes env.sh and interactive.sh and hooks them into the startup
// files of the shells the description names. Without a shell section it
// does nothing.
func ApplyShell(homeDir string, c *config.Config) ([]Change, error) {
	if c.Shell == nil {
		return nil, nil
	}
	env, err := shell.Env(c.Shell)
	if err != nil {
		return nil, err
	}
	interactive, err := shell.Interactive(c.Shell, len(c.Tools) > 0)
	if err != nil {
		return nil, err
	}

	var changes []Change
	for _, f := range []struct{ name, content string }{
		{shell.EnvFile, env},
		{shell.InteractiveFile, interactive},
	} {
		rel := filepath.Join(ConfigDir, f.name)
		wrote, err := startup.WriteFile(filepath.Join(homeDir, rel), f.content, 0o644)
		if err != nil {
			return changes, err
		}
		if wrote {
			changes = append(changes, Change{rel, "written"})
		}
	}

	var errs []error
	hook := func(rel string, l startup.Line, extra ...string) {
		r, err := startup.Ensure(filepath.Join(homeDir, rel), l, extra...)
		if err != nil {
			errs = append(errs, err)
			return
		}
		if r != startup.Unchanged {
			changes = append(changes, Change{rel, fmt.Sprintf("line %s", r)})
		}
	}
	envLine := startup.SourceLine(shell.EnvFile)
	rcLine := startup.SourceLine(shell.InteractiveFile)
	for _, sh := range c.Shell.Shells {
		switch sh {
		case "bash":
			login, err := bashLoginFile(homeDir)
			if err != nil {
				errs = append(errs, err)
			} else if login == "" {
				hook(".bash_profile", envLine, bashrcFromProfile)
			} else {
				hook(login, envLine)
			}
			hook(".bashrc", rcLine)
		case "zsh":
			hook(".zshenv", envLine)
			hook(".zshrc", rcLine)
		}
	}
	return changes, errors.Join(errs...)
}

// bashLoginFile returns the login file bash would read, or "" if none
// exists.
func bashLoginFile(homeDir string) (string, error) {
	for _, name := range bashLoginFiles {
		_, err := os.Lstat(filepath.Join(homeDir, name))
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return "", nil
}
