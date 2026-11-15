// Command hiwthi sets up a home directory from a description kept in git.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/waco-org/hiwthi/internal/config"
	"github.com/waco-org/hiwthi/internal/home"
)

const usage = `Usage: hiwthi <command> [options]

Commands:
  apply [-repo DIR]   make the home match the description (so far: the shell)
  config [-repo DIR]  show the merged description (common.yml + local.yml)
  version             show Hiwthi's version

DIR defaults to ~/.local/share/hiwthi/repo.

Planned: init, update, diff, check, capture.
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
	case "apply":
		return runApply(args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "version":
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
	changes, err := home.ApplyShell(h, c)
	for _, ch := range changes {
		fmt.Fprintf(stdout, "~/%s: %s\n", ch.Path, ch.What)
	}
	if err != nil {
		fmt.Fprintln(stderr, "hiwthi:", err)
		return 1
	}
	return 0
}
