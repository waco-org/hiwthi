// Command hiwthi sets up a home directory from a description kept in git.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/waco-org/hiwthi/internal/config"
	"github.com/waco-org/hiwthi/internal/home"
	"github.com/waco-org/hiwthi/internal/repo"
)

const usage = `Usage: hiwthi <command> [options]

Commands:
  init [-repo DIR] [-branch NAME] [-no-apply] URL
                      clone your Hiwthi repo (the branch for this kind of
                      machine), then apply it
  apply [-repo DIR]   make the home match the description (so far: files/, shell, identity)
  config [-repo DIR]  show the merged description (common.yml + local.yml)
  version             show Hiwthi's version

DIR defaults to ~/.local/share/hiwthi/repo.

Planned: update, diff, check, capture.
`

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one command and returns the exit status.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "apply":
		return runApply(args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, "hiwthi", version)
		return 0
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "hiwthi: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	repo, ok := parseRepo("config", args, stderr)
	if !ok {
		return 2
	}

	// Load checks everything; LoadNode gives the merged text to show.
	if _, err := config.Load(repo); err != nil {
		fmt.Fprintln(stderr, "hiwthi:", err)
		return 1
	}
	merged, err := config.LoadNode(repo)
	if err != nil {
		fmt.Fprintln(stderr, "hiwthi:", err)
		return 1
	}
	enc := yaml.NewEncoder(stdout)
	enc.SetIndent(2)
	if err := enc.Encode(merged); err != nil {
		fmt.Fprintln(stderr, "hiwthi:", err)
		return 1
	}
	return 0
}

// defaultRepo is where hiwthi init will clone the user's repo, relative to
// the home.
const defaultRepo = ".local/share/hiwthi/repo"

// parseRepo parses the options shared by the commands that read the repo.
func parseRepo(cmd string, args []string, stderr io.Writer) (string, bool) {
	flags := flag.NewFlagSet(cmd, flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", "", "the directory holding common.yml and local.yml\n(default ~/"+defaultRepo+")")
	if err := flags.Parse(args); err != nil {
		return "", false
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "hiwthi %s: unexpected argument %q\n", cmd, flags.Arg(0))
		return "", false
	}
	if *repo == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(stderr, "hiwthi:", err)
			return "", false
		}
		*repo = filepath.Join(h, defaultRepo)
	}
	return *repo, true
}

func runApply(args []string, stdout, stderr io.Writer) int {
	repo, ok := parseRepo("apply", args, stderr)
	if !ok {
		return 2
	}
	return apply(repo, stdout, stderr)
}

// apply makes the home match the description in repo.
func apply(repo string, stdout, stderr io.Writer) int {
	c, err := config.Load(repo)
	if err != nil {
		fmt.Fprintln(stderr, "hiwthi:", err)
		return 1
	}
	h, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "hiwthi:", err)
		return 1
	}
	// files/ first: the other parts then find the startup files it brings.
	backup := filepath.Join(home.BackupDir, time.Now().Format("20060102-150405"))
	applyFiles := func(h string, c *config.Config) ([]home.Change, error) {
		return home.ApplyFiles(h, repo, backup, c)
	}
	// Each part goes on when another fails: they touch different files.
	status := 0
	for _, part := range []func(string, *config.Config) ([]home.Change, error){
		applyFiles,
		home.ApplyShell,
		home.ApplyIdentity,
	} {
		changes, err := part(h, c)
		for _, ch := range changes {
			fmt.Fprintf(stdout, "~/%s: %s\n", ch.Path, ch.What)
		}
		if err != nil {
			fmt.Fprintln(stderr, "hiwthi:", err)
			status = 1
		}
	}
	return status
}

func runInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("repo", "", "where to clone (default ~/"+defaultRepo+")")
	branch := flags.String("branch", "", "the branch to check out (default: the remote's)")
	noApply := flags.Bool("no-apply", false, "clone only, don't apply")
	// Options may also come after the URL: parse again after it.
	var urls []string
	for {
		if err := flags.Parse(args); err != nil {
			return 2
		}
		if flags.NArg() == 0 {
			break
		}
		urls = append(urls, flags.Arg(0))
		args = flags.Args()[1:]
	}
	if len(urls) != 1 {
		fmt.Fprintln(stderr, "hiwthi init: expected one URL, your Hiwthi repo")
		return 2
	}
	if *dir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(stderr, "hiwthi:", err)
			return 1
		}
		*dir = filepath.Join(h, defaultRepo)
	}

	err := repo.Clone(repo.CloneOptions{URL: urls[0], Dir: *dir, Branch: *branch, Output: stderr})
	switch {
	case errors.Is(err, repo.ErrAlreadyCloned):
		fmt.Fprintf(stdout, "%s already holds %s\n", *dir, urls[0])
	case err != nil:
		fmt.Fprintln(stderr, "hiwthi:", err)
		return 1
	default:
		fmt.Fprintf(stdout, "cloned %s into %s\n", urls[0], *dir)
	}
	if *noApply {
		return 0
	}
	return apply(*dir, stdout, stderr)
}
