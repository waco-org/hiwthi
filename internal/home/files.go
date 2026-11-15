package home

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"filippo.io/age"

	"github.com/waco-org/hiwthi/internal/config"
	"github.com/waco-org/hiwthi/internal/shell"
	"github.com/waco-org/hiwthi/internal/startup"
)

// FilesDir is the directory in the user's repo whose content is copied into
// the home.
const FilesDir = "files"

// KeyFile holds this machine's age identities, relative to the home: the
// key that decrypts the repo's .age files. Copied to each machine by hand;
// never in the repo.
const KeyFile = ConfigDir + "/key.txt"

// BackupDir is where files Hiwthi replaces are saved, relative to the home,
// one directory per apply.
const BackupDir = ".local/state/hiwthi/backup"

// privateDirs are created with mode 0700 when Hiwthi creates them.
var privateDirs = []string{".ssh", ".gnupg"}

// owned are the files Hiwthi generates: files/ can't provide them.
var owned = []string{shell.EnvFile, shell.InteractiveFile, GitFile, HgFile, "key.txt"}

// entry is one file of files/, as it will be in the home.
type entry struct {
	rel     string // in the home
	content []byte
	mode    fs.FileMode
	link    string // the target, for a symbolic link
}

// ApplyFiles copies the repo's files/ into the home. Files ending in .age are
// decrypted with the machine's key (KeyFile) and written without the
// suffix, readable only by the user; without a key they are skipped. A
// startup file Hiwthi hooks into gets its line as it is copied, so the copy
// and the hook agree. A file that differs is saved under backup before it is
// replaced; backup is relative to the home.
func ApplyFiles(homeDir, repoDir, backup string, c *config.Config) ([]Change, error) {
	identities, err := loadKey(homeDir)
	if err != nil {
		return nil, err
	}
	entries, errs := readFiles(filepath.Join(repoDir, FilesDir), identities)

	// The hooks are planned as they will be once the files are copied.
	present := map[string]bool{}
	for _, e := range entries {
		present[e.rel] = true
	}
	home := inHome(homeDir)
	exists := func(rel string) bool { return present[rel] || home(rel) }
	hooks := map[string][]hookSpec{}
	for _, h := range append(shellHooks(c, exists), identityHooks(c, exists)...) {
		hooks[h.rel] = append(hooks[h.rel], h)
	}

	var changes []Change
	for _, e := range entries {
		if e.link == "" {
			text := string(e.content)
			var err error
			for _, h := range hooks[e.rel] {
				text, _, err = startup.EnsureText(text, false, h.line)
				if err != nil {
					break
				}
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s/%s: %w", FilesDir, e.rel, err))
				continue
			}
			e.content = []byte(text)
		}
		ch, err := install(homeDir, backup, e)
		changes = append(changes, ch...)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return changes, errors.Join(errs...)
}

// loadKey reads the machine's age identities; none if there is no key file.
func loadKey(homeDir string) ([]age.Identity, error) {
	f, err := os.Open(filepath.Join(homeDir, KeyFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ids, err := age.ParseIdentities(f)
	if err != nil {
		return nil, fmt.Errorf("~/%s: %w", KeyFile, err)
	}
	return ids, nil
}

// readFiles reads files/ into entries. A missing files/ is empty. Errors
// concern single files, which are left out.
func readFiles(dir string, identities []age.Identity) ([]entry, []error) {
	var entries []entry
	var errs []error
	seen := map[string]string{} // home name → name in files/
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		src, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel := strings.TrimSuffix(src, ".age")
		fail := func(err error) {
			errs = append(errs, fmt.Errorf("%s/%s: %w", FilesDir, src, err))
		}
		if filepath.Dir(rel) == ConfigDir && slices.Contains(owned, filepath.Base(rel)) {
			fail(errors.New("Hiwthi writes this file itself"))
			return nil
		}
		if other, ok := seen[rel]; ok {
			fail(fmt.Errorf("both it and %s/%s become ~/%s", FilesDir, other, rel))
			return nil
		}

		e := entry{rel: rel}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			if rel != src {
				fail(errors.New("an encrypted file can't be a symbolic link"))
				return nil
			}
			if e.link, err = os.Readlink(path); err != nil {
				return err
			}
		case !info.Mode().IsRegular():
			fail(errors.New("not a regular file"))
			return nil
		case rel != src: // .age
			if len(identities) == 0 {
				return nil // no key on this machine: skipped silently
			}
			if e.content, err = decrypt(path, identities); err != nil {
				fail(err)
				return nil
			}
			e.mode = 0o600
		default:
			if e.content, err = os.ReadFile(path); err != nil {
				return err
			}
			e.mode = 0o644
			if info.Mode()&0o111 != 0 {
				e.mode = 0o755
			}
		}
		seen[rel] = src
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	return entries, errs
}

func decrypt(path string, identities []age.Identity) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r, err := age.Decrypt(f, identities...)
	if err != nil {
		return nil, fmt.Errorf("decrypting with ~/%s: %w", KeyFile, err)
	}
	return io.ReadAll(r)
}

// install puts one entry in the home, if it differs from what is there.
func install(homeDir, backup string, e entry) ([]Change, error) {
	path := filepath.Join(homeDir, e.rel)
	fail := func(err error) ([]Change, error) { return nil, fmt.Errorf("~/%s: %w", e.rel, err) }

	info, err := os.Lstat(path)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return fail(err)
	}

	// A file kept as a symbolic link by another tool is written where it
	// points; a symbolic link from files/ replaces whatever is there.
	if e.link == "" && !missing && info.Mode()&fs.ModeSymlink != 0 {
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
			info, err = os.Lstat(path)
			if err != nil {
				return fail(err)
			}
		}
	}

	if !missing {
		if same(path, info, e) {
			return nil, nil
		}
		if info.IsDir() {
			return fail(errors.New("a directory is in the way"))
		}
	}
	if err := mkdirs(homeDir, filepath.Dir(e.rel)); err != nil {
		return fail(err)
	}

	what := "created"
	if !missing {
		saved := filepath.Join(backup, e.rel)
		if err := save(path, filepath.Join(homeDir, saved)); err != nil {
			return fail(fmt.Errorf("saving the old copy: %w", err))
		}
		what = "replaced, old copy in ~/" + saved
	}
	if e.link != "" {
		tmp := path + ".hiwthi-link"
		os.Remove(tmp)
		if err := os.Symlink(e.link, tmp); err != nil {
			return fail(err)
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
			return fail(err)
		}
	} else if _, err := startup.WriteFile(path, string(e.content), e.mode); err != nil {
		return fail(err)
	}
	return []Change{{e.rel, what}}, nil
}

// same tells whether the file at path already is e.
func same(path string, info fs.FileInfo, e entry) bool {
	if e.link != "" {
		target, err := os.Readlink(path)
		return err == nil && target == e.link
	}
	if !info.Mode().IsRegular() {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && bytes.Equal(data, e.content)
}

// save copies the file or link at path to dst, creating dst's directory.
func save(path, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
}

// mkdirs creates dir, relative to the home, and its parents; those named in
// privateDirs get mode 0700.
func mkdirs(homeDir, dir string) error {
	if dir == "." {
		return nil
	}
	path := homeDir
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		mode := fs.FileMode(0o755)
		if slices.Contains(privateDirs, part) {
			mode = 0o700
		}
		if err := os.Mkdir(path, mode); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return nil
}
