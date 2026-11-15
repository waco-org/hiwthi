package home

import (
	"errors"

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
	for _, h := range shellHooks(c, inHome(homeDir)) {
		ch, err := hook(homeDir, h)
		changes = append(changes, ch...)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return changes, errors.Join(errs...)
}

// shellHooks plans the lines for the shells the description names. exists
// tells whether a file, relative to the home, exists.
func shellHooks(c *config.Config, exists func(string) bool) []hookSpec {
	if c.Shell == nil {
		return nil
	}
	envLine := startup.SourceLine(shell.EnvFile)
	rcLine := startup.SourceLine(shell.InteractiveFile)
	var hooks []hookSpec
	for _, sh := range c.Shell.Shells {
		switch sh {
		case "bash":
			if login := bashLoginFile(exists); login == "" {
				hooks = append(hooks, hookSpec{".bash_profile", envLine, []string{bashrcFromProfile}})
			} else {
				hooks = append(hooks, hookSpec{login, envLine, nil})
			}
			hooks = append(hooks, hookSpec{".bashrc", rcLine, nil})
		case "zsh":
			hooks = append(hooks, hookSpec{".zshenv", envLine, nil}, hookSpec{".zshrc", rcLine, nil})
		}
	}
	return hooks
}

// bashLoginFile returns the login file bash would read, or "" if none
// exists.
func bashLoginFile(exists func(string) bool) string {
	for _, name := range bashLoginFiles {
		if exists(name) {
			return name
		}
	}
	return ""
}
