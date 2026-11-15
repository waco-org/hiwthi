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
// points to ("env.sh"), Text the whole line Hiwthi writes.
type Line struct {
	Target string
	Text   string
}

// SourceLine is the line that sources ~/.config/hiwthi/<target> from a shell
// startup file.
func SourceLine(target string) Line {
	p := "~/.config/hiwthi/" + target
	return Line{Target: target, Text: fmt.Sprintf("[ -r %s ] && . %s", p, p)}
}

// matches tells whether a line of a file is Hiwthi's line for target.
// Comments don't count, so the comment above the line, or a line the user
// commented out, isn't taken for it.
func matches(line, target string) bool {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "#") {
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

	data, err := os.ReadFile(real)
	if errors.Is(err, fs.ErrNotExist) {
		content := Comment + "\n" + l.Text + "\n"
		for _, e := range extra {
			content += e + "\n"
		}
		if err := writeFile(real, content, 0o644); err != nil {
			return Unchanged, err
		}
		return Created, nil
	}
	if err != nil {
		return Unchanged, err
	}

	text := string(data)
	lines := strings.SplitAfter(text, "\n")
	found := -1
	for i, line := range lines {
		if matches(line, l.Target) {
			if found >= 0 {
				return Unchanged, fmt.Errorf("%s: %w ~/.config/hiwthi/%s (lines %d and %d)",
					path, ErrSeveral, l.Target, found+1, i+1)
			}
			found = i
		}
	}

	var result Result
	switch {
	case found < 0:
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "\n" + Comment + "\n" + l.Text + "\n"
		result = Added
	case strings.TrimRight(lines[found], "\r\n") == l.Text:
		return Unchanged, nil
	default:
		ending := lines[found][len(strings.TrimRight(lines[found], "\r\n")):]
		lines[found] = l.Text + ending
		text = strings.Join(lines, "")
		result = Replaced
	}

	info, err := os.Stat(real)
	if err != nil {
		return Unchanged, err
	}
	if err := writeFile(real, text, info.Mode().Perm()); err != nil {
		return Unchanged, err
	}
	return result, nil
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
