package identity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kballard/go-shellquote"
)

type stagedFile struct{ original, temporary string }

func stageRemoval(path string) (stagedFile, error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".gh-identity-delete-*")
	if err != nil {
		return stagedFile{}, err
	}
	if err := f.Close(); err != nil {
		return stagedFile{}, errors.Join(err, os.Remove(f.Name()))
	}
	if err := os.Rename(path, f.Name()); err != nil {
		return stagedFile{}, errors.Join(err, os.Remove(f.Name()))
	}
	return stagedFile{path, f.Name()}, nil
}

func restoreFiles(files []stagedFile) error {
	var errs []error
	for _, f := range files {
		if err := os.Rename(f.temporary, f.original); err != nil {
			errs = append(errs, fmt.Errorf("restore %s from %s: %w", f.original, f.temporary, err))
		}
	}
	return errors.Join(errs...)
}

func ownedFile(path, marker string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to delete non-regular managed file %s; inspect it manually", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(data), marker+"\n") {
		return fmt.Errorf("cannot confirm gh owns %s; inspect it manually before removing it", path)
	}
	return nil
}

func (s *Store) validateManagedIntegration() error {
	path := s.authConfigPath()
	if err := ownedFile(path, "# Managed by gh auth use. Do not edit."); err != nil {
		return err
	}
	out, err := gitOutput("", "config", "--file", path, "--no-includes", "--null", "--list")
	if err != nil {
		return err
	}
	for entry := range strings.SplitSeq(strings.TrimSuffix(out, "\x00"), "\x00") {
		if entry == "" {
			continue
		}
		key, value, _ := strings.Cut(entry, "\n")
		managed := false
		switch {
		case strings.HasPrefix(key, "gh-auth-binding."):
			managed = true // Bindings validates the supported metadata fields.
		case key == "core.sshcommand":
			args, err := shellquote.Split(value)
			managed = err == nil && isSSHDispatcher(args)
		case key == "ssh.variant":
			managed = value == "ssh"
		case strings.HasPrefix(key, "credential.https://"):
			if strings.HasSuffix(key, ".usehttppath") {
				managed = value == "true"
			} else if strings.HasSuffix(key, ".helper") {
				if value == "" {
					managed = true
				} else if strings.HasPrefix(value, "!") {
					args, err := shellquote.Split(value[1:])
					managed = err == nil && len(args) == 3 && args[1] == "auth" && args[2] == "git-credential"
				}
			}
		case strings.HasPrefix(key, "includeif.") && strings.HasSuffix(key, ".path"):
			managed = filepath.IsAbs(value) && filepath.Clean(filepath.Dir(value)) == filepath.Clean(s.Dir) && strings.HasSuffix(value, ".gitconfig")
		}
		if !managed {
			return fmt.Errorf("%s contains settings not managed by gh (%s); move those settings to your own Git configuration before retrying cleanup", path, key)
		}
	}
	return nil
}

// DeleteIdentities removes the named identities and their bindings. All also
// removes orphaned bindings and unused managed integration. Credentials are untouched.
func (s *Store) DeleteIdentities(names []string, all bool, executable string) error {
	bindings, err := s.Bindings()
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, err := s.Load(name); err != nil {
			return err
		}
		if err := ownedFile(s.IdentityPath(name), "# Managed by gh auth identity. Manual changes may be overwritten."); err != nil {
			return err
		}
	}
	remaining := slices.DeleteFunc(slices.Clone(bindings), func(b Binding) bool {
		return all || slices.Contains(names, b.Identity)
	})
	// Validate before staging identities so configuration errors leave them in place.
	if _, err := os.Stat(s.authConfigPath()); err == nil {
		if err := s.validateManagedIntegration(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var staged []stagedFile
	for _, name := range names {
		file, err := stageRemoval(s.IdentityPath(name))
		if err != nil {
			return errors.Join(err, restoreFiles(staged))
		}
		staged = append(staged, file)
	}
	if len(remaining) == 0 {
		err = s.clearBindings()
	} else if len(remaining) != len(bindings) {
		err = s.writeBindings(remaining, executable)
	}
	if err != nil {
		return errors.Join(err, restoreFiles(staged))
	}
	var errs []error
	for _, file := range staged {
		if err := os.Remove(file.temporary); err != nil {
			errs = append(errs, fmt.Errorf("identity removed, but temporary file %s could not be deleted: %w", file.temporary, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(s.Dir)
	}
	return nil
}

func (s *Store) clearBindings() error {
	path := s.authConfigPath()
	var staged []stagedFile
	if err := s.validateManagedIntegration(); err == nil {
		file, err := stageRemoval(path)
		if err != nil {
			return err
		}
		staged = append(staged, file)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	paths, err := gitOptional("", "config", "--global", "--get-all", "include.path")
	if err != nil {
		return errors.Join(err, restoreFiles(staged))
	}
	if slices.Contains(strings.Split(paths, "\n"), path) {
		if _, err := gitOutput("", "config", "--global", "--fixed-value", "--unset-all", "include.path", path); err != nil {
			return errors.Join(err, restoreFiles(staged))
		}
	}
	for _, file := range staged {
		if err := os.Remove(file.temporary); err != nil {
			return fmt.Errorf("Git integration detached, but could not remove %s: %w", file.temporary, err)
		}
	}
	return nil
}
