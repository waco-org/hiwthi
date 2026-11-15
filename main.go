// Command hiwthi sets up a home directory from a description kept in git.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"go.yaml.in/yaml/v3"

	"github.com/waco-org/hiwthi/internal/config"
)

const usage = `Usage: hiwthi <command> [options]

Commands:
  config [-repo DIR]  show the merged description (common.yml + local.yml)
  version             show Hiwthi's version

Planned: init, apply, update, diff, check, capture.
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
	flags := flag.NewFlagSet("config", flag.ContinueOnError)
	flags.SetOutput(stderr)
	// The default repo location is still to be decided; for now the current
	// directory.
	repo := flags.String("repo", ".", "the directory holding common.yml and local.yml")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	// Load checks everything; LoadNode gives the merged text to show.
	if _, err := config.Load(*repo); err != nil {
		fmt.Fprintln(stderr, "hiwthi:", err)
		return 1
	}
	merged, err := config.LoadNode(*repo)
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
