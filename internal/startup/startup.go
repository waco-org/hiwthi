// Package startup keeps Hiwthi's one line in files it doesn't own: the shell
// startup files (and later .gitconfig and .hgrc). The line is recognised by
// what it points to, a file in ~/.config/hiwthi, not by a comment marker.
package startup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Comment is written above the line when Hiwthi adds it. Only for the
// reader: Hiwthi doesn't need it to find the line.
const Comment = "# Added by hiwthi; see hiwthi check"

// Result says what Ensure did.
type Result int

const (
	Unchanged Result = iota
	Created          // the file didn't exist
	Added            // the line was appended
	Replaced         // a different line pointing at the same target was replaced
)

func (r Result) String() string {
	return [...]string{"unchanged", "created", "added", "replaced"}[r]
}

// ErrSeveral is returned when a file has more than one line pointing at the
// target: Hiwthi can't tell which one to keep, so it leaves the file alone.
var ErrSeveral = errors.New("more than one line points at")

// Line is one marked line: Target is the file under ~/.config/hiwthi it
// points to ("env.sh"), Text the whole line Hiwthi writes. Header, if set,
// is written before the line when Hiwthi adds it (gitconfig's "[include]").
type Line struct {
	Target string
	Text   string
	Header string
}

// added is what Ensure writes when the line is missing.
func (l Line) added() string {
	s := Comment + "\n"
	if l.Header != "" {
		s += l.Header + "\n"
	}
	return s + l.Text + "\n"
}

// GitInclude is the include of ~/.config/hiwthi/<target> in a git
// configuration file.
func GitInclude(target string) Line {
	return Line{Target: target, Header: "[include]",
		Text: "\tpath = ~/.config/hiwthi/" + target}
}

// HgInclude is the include of ~/.config/hiwthi/<target> in a Mercurial
// configuration file.
func HgInclude(target string) Line {
	return Line{Target: target, Text: "%include ~/.config/hiwthi/" + target}
}

// SourceLine is the line that sources ~/.config/hiwthi/<target> from a shell
// startup file.
func SourceLine(target string) Line {
	p := "~/.config/hiwthi/" + target
	return Line{Target: target, Text: fmt.Sprintf("[ -r %s ] && . %s", p, p)}
}

// matches tells whether a line of a file is Hiwthi's line for target.
// Comments (# in all the files Hiwthi touches, also ; in git's and
// Mercurial's) don't count, so the comment above the line, or a line the user
// commented out, isn't taken for it.
func matches(line, target string) bool {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "#") || strings.HasPrefix(t, ";") {
		return false
	}
	return strings.Contains(t, ".config/hiwthi/"+target)
}

// Ensure makes path contain l.Text exactly once. A symbolic link is followed,
// so a startup file kept by another dotfile manager is changed where it
// lives. extra is written after the line when the file is created.
func Ensure(path string, l Line, extra ...string) (Result, error) {
	real, err := filepath.EvalSymlinks(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		real = path
	case err != nil:
		return Unchanged, err
	}

	perm := fs.FileMode(0o644)
	data, err := os.ReadFile(real)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return Unchanged, err
	}
	if !missing {
		info, err := os.Stat(real)
		if err != nil {
			return Unchanged, err
		}
		perm = info.Mode().Perm()
	}
	text, result, err := EnsureText(string(data), missing, l, extra...)
	if err != nil {
		return Unchanged, fmt.Errorf("%s: %w", path, err)
	}
	if result == Unchanged {
		return Unchanged, nil
	}
	if err := writeFile(real, text, perm); err != nil {
		return Unchanged, err
	}
	return result, nil
}

// EnsureText is Ensure on a file's content: it returns text with l.Text
// exactly once. missing says the file doesn't exist (text is then empty); it
// gets the line and extra.
func EnsureText(text string, missing bool, l Line, extra ...string) (string, Result, error) {
	if missing {
		content := l.added()
		for _, e := range extra {
			content += e + "\n"
		}
		return content, Created, nil
	}

	lines := strings.SplitAfter(text, "\n")
	found := -1
	for i, line := range lines {
		if matches(line, l.Target) {
			if found >= 0 {
				return text, Unchanged, fmt.Errorf("%w ~/.config/hiwthi/%s (lines %d and %d)",
					ErrSeveral, l.Target, found+1, i+1)
			}
			found = i
		}
	}

	switch {
	case found < 0:
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return text + "\n" + l.added(), Added, nil
	case strings.TrimSpace(lines[found]) == strings.TrimSpace(l.Text):
		// Indentation is the user's business.
		return text, Unchanged, nil
	default:
		ending := lines[found][len(strings.TrimRight(lines[found], "\r\n")):]
		lines[found] = l.Text + ending
		return strings.Join(lines, ""), Replaced, nil
	}
}

// WriteFile writes content to path if it differs from what is there,
// creating the directory, and tells whether it wrote.
func WriteFile(path, content string, perm fs.FileMode) (bool, error) {
	old, err := os.ReadFile(path)
	if err == nil && string(old) == content {
		return false, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, writeFile(path, content, perm)
}

// writeFile replaces path atomically: a temporary file in the same
// directory, renamed over it.
func writeFile(path, content string, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".hiwthi-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // fails harmlessly after the rename
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
