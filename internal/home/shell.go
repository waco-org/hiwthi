package home

import (
	"errors"
	"path/filepath"

	"github.com/waco-org/hiwthi/internal/config"
	"github.com/waco-org/hiwthi/internal/shell"
	"github.com/waco-org/hiwthi/internal/startup"
)

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
	var errs []error
	for _, f := range []struct{ name, content string }{
		{shell.EnvFile, env},
		{shell.InteractiveFile, interactive},
	} {
		ch, err := writeOwned(homeDir, f.name, f.content)
		changes = append(changes, ch...)
		if err != nil {
			return changes, err
		}
	}
	add := func(rel string, l startup.Line, extra ...string) {
		ch, err := hook(homeDir, rel, l, extra...)
		changes = append(changes, ch...)
		if err != nil {
			errs = append(errs, err)
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
				add(".bash_profile", envLine, bashrcFromProfile)
			} else {
				add(login, envLine)
			}
			add(".bashrc", rcLine)
		case "zsh":
			add(".zshenv", envLine)
			add(".zshrc", rcLine)
		}
	}
	return changes, errors.Join(errs...)
}

// bashLoginFile returns the login file bash would read, or "" if none
// exists.
func bashLoginFile(homeDir string) (string, error) {
	for _, name := range bashLoginFiles {
		ok, err := exists(filepath.Join(homeDir, name))
		if err != nil || ok {
			return name, err
		}
	}
	return "", nil
}
