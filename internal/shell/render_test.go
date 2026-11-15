package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waco-org/hiwthi/internal/config"
)

// run sources script in the given shell with HOME set to home and a minimal
// PATH, then runs cmd, and returns its output. Skips if the shell is
// missing.
func run(t *testing.T, sh, home, script, cmd string, interactive bool) string {
	t.Helper()
	bin, err := exec.LookPath(sh)
	if err != nil {
		t.Skipf("%s not installed", sh)
	}
	file := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--norc", "--noprofile"} // bash wants them first
	if sh == "zsh" {
		args = []string{"-f"} // no startup files
	}
	if interactive {
		args = append(args, "-i")
	}
	args = append(args, "-c", ". "+file+"; "+cmd)
	c := exec.Command(bin, args...)
	c.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "TERM=dumb"}
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", sh, err, out)
	}
	// bash -i without a terminal complains about job control.
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(l, "bash: ") {
			lines = append(lines, l)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnvPath(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".local/bin", "bin", "with space", "late")
	env, err := Env(&config.Shell{
		Path:       []string{"~/.local/bin", "~/missing", "~/bin", "~/with space"},
		PathAppend: []string{"~/late", "/nonexistent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		home + "/.local/bin", home + "/bin", home + "/with space",
		"/usr/bin", "/bin", home + "/late"}, ":")
	for _, sh := range []string{"bash", "zsh"} {
		// Sourced twice: the second time changes nothing.
		got := run(t, sh, home, env+"\n"+env, `echo "$PATH"`, false)
		if got != want {
			t.Errorf("%s: PATH = %q\nwant %q", sh, got, want)
		}
	}
}

func TestEnvEnvironment(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, "jdk")
	env, err := Env(&config.Shell{Environment: map[string]string{
		"JAVA_HOME": "~/jdk",
		"GONE":      "/nonexistent",
		"EDITOR":    "vim -p",
		"QUOTED":    `it's $HOME`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := home + "/jdk|unset|vim -p|it's $HOME"
	for _, sh := range []string{"bash", "zsh"} {
		got := run(t, sh, home, env,
			`echo "$JAVA_HOME|${GONE-unset}|$EDITOR|$QUOTED"`, false)
		if got != want {
			t.Errorf("%s: got %q, want %q", sh, got, want)
		}
	}
}

func TestEnvRejects(t *testing.T) {
	for _, s := range []*config.Shell{
		{Path: []string{"relative/dir"}},
		{Path: []string{"~other/bin"}},
		{Environment: map[string]string{"BAD-NAME": "x"}},
	} {
		if _, err := Env(s); err == nil {
			t.Errorf("Env(%+v) succeeded, expected an error", s)
		}
	}
}

func TestInteractive(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".config/hiwthi")
	for _, sh := range []string{"bash", "zsh"} {
		local := filepath.Join(home, ".config/hiwthi/local."+sh)
		if err := os.WriteFile(local, []byte("LOCAL=yes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script, err := Interactive(&config.Shell{
		EditingMode: "vi",
		Aliases:     map[string]string{"ll": "ls -l", "q": `echo 'x'`},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		"bash": `set -o | grep -w vi | tr -s ' \t' ' '; alias ll q; echo $LOCAL`,
		"zsh":  `bindkey -lL main; alias ll q; echo $LOCAL`,
	}
	wants := map[string]string{
		"bash": "vi on\nalias ll='ls -l'\nalias q='echo '\\''x'\\'''\nyes",
		"zsh":  "bindkey -A viins main\nll='ls -l'\nq='echo '\\''x'\\'\nyes", // zsh's quoting
	}
	for sh, cmd := range checks {
		if got := run(t, sh, home, script, cmd, true); got != wants[sh] {
			t.Errorf("%s: got\n%s\nwant\n%s", sh, got, wants[sh])
		}
	}
}

func TestInteractiveDoesNothingWhenNotInteractive(t *testing.T) {
	script, err := Interactive(&config.Shell{Aliases: map[string]string{"ll": "ls -l"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, sh := range []string{"bash", "zsh"} {
		if got := run(t, sh, t.TempDir(), script, "alias ll 2>/dev/null || echo none", false); got != "none" {
			t.Errorf("%s: got %q, want none", sh, got)
		}
	}
}

func TestInteractiveMise(t *testing.T) {
	without, _ := Interactive(&config.Shell{}, false)
	with, _ := Interactive(&config.Shell{}, true)
	if strings.Contains(without, "mise") || !strings.Contains(with, "mise activate zsh") {
		t.Error("mise activation should appear only with tools")
	}
}
