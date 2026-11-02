// Package identity manages experimental, reusable GitHub and Git identities.
package identity

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cli/cli/v2/git"
	ghconfig "github.com/cli/go-gh/v2/pkg/config"
	"github.com/kballard/go-shellquote"
)

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Identity is a saved account reference and optional Git preferences, never credentials.
type Identity struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	Account      string `json:"account"`
	SSHHost      string `json:"sshHost,omitempty"`
	GitUserName  string `json:"gitUserName,omitempty"`
	GitUserEmail string `json:"gitUserEmail,omitempty"`
}

// Binding applies an identity to a repository family, directory tree, or owner.
type Binding struct {
	Identity   string `json:"identity"`
	Host       string `json:"host"`
	Directory  string `json:"directory,omitempty"`
	Repository string `json:"repository,omitempty"` // Canonical common Git directory.
	Owner      string `json:"owner,omitempty"`
}

// Store owns the generated identity files and the scoped references to them.
type Store struct {
	Dir string
}

// Open locates identity storage without creating files or changing Git configuration.
func Open() (*Store, error) {
	dir, err := filepath.Abs(filepath.Join(ghconfig.ConfigDir(), "identities"))
	if err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

// IdentityPath returns the managed Git config path for a validated identity name.
func (s *Store) IdentityPath(name string) string {
	return filepath.Join(s.Dir, name+".gitconfig")
}

func validValue(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func gitQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`, "\b", `\b`).Replace(value) + `"`
}

// Save creates or replaces a managed identity file. Editing preserves its references.
func (s *Store) Save(id Identity, replace bool) error {
	if !safeName.MatchString(id.Name) || !safeName.MatchString(id.Host) || !safeName.MatchString(id.Account) {
		return errors.New("identity name, hostname, and account must contain only letters, digits, dots, underscores, and hyphens, starting with a letter or digit")
	}
	if id.SSHHost != "" && !safeName.MatchString(id.SSHHost) {
		return errors.New("SSH host must be an existing SSH host name or alias, not a URL or command")
	}
	if !validValue(id.GitUserName) || !validValue(id.GitUserEmail) {
		return errors.New("Git name and email must be valid UTF-8 without newlines or NUL characters")
	}
	if (id.GitUserName == "") != (id.GitUserEmail == "") {
		return errors.New("supply both --git-user-name and --git-user-email, or neither")
	}
	path := s.IdentityPath(id.Name)
	if _, err := os.Stat(path); err == nil && !replace {
		return fmt.Errorf("identity %q already exists; use 'gh auth identity edit'", id.Name)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if replace {
		old, err := s.Load(id.Name)
		if err != nil {
			return err
		}
		if old.Host != id.Host {
			return errors.New("an identity's hostname cannot change; create another identity")
		}
	}
	if id.GitUserName != "" {
		bindings, err := s.Bindings()
		if err != nil {
			return err
		}
		if err := s.validateDirectoryAttribution(bindings, &id); err != nil {
			return err
		}
	}
	var b strings.Builder
	fmt.Fprintln(&b, "# Managed by gh auth identity. Manual changes may be overwritten.")
	fmt.Fprintf(&b, "[gh-identity %s]\n", gitQuote(id.Host))
	for _, field := range []struct{ key, value string }{
		{"name", id.Name}, {"account", id.Account}, {"sshHost", id.SSHHost},
		{"gitUserName", id.GitUserName}, {"gitUserEmail", id.GitUserEmail},
	} {
		fmt.Fprintf(&b, "\t%s = %s\n", field.key, gitQuote(field.value))
	}
	if id.GitUserName != "" {
		fmt.Fprintf(&b, "[user]\n\tname = %s\n\temail = %s\n", gitQuote(id.GitUserName), gitQuote(id.GitUserEmail))
	}
	fmt.Fprintf(&b, "[credential %s]\n\tusername = %s\n", gitQuote("https://"+id.Host), gitQuote(id.Account))
	return atomicWrite(path, []byte(b.String()))
}

// Load reads only the supported identity fields from its managed Git config.
func (s *Store) Load(name string) (*Identity, error) {
	if !safeName.MatchString(name) {
		return nil, fmt.Errorf("invalid identity name %q", name)
	}
	path := s.IdentityPath(name)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("identity %q: %w", name, err)
	}
	out, err := gitOutput("", "config", "--no-includes", "--file", path, "--null", "--list")
	if err != nil {
		return nil, err
	}
	id := &Identity{}
	for entry := range strings.SplitSeq(out, "\x00") {
		key, value, ok := strings.Cut(entry, "\n")
		if !ok || !strings.HasPrefix(key, "gh-identity.") {
			continue
		}
		field := strings.LastIndexByte(key, '.')
		if field <= len("gh-identity.") {
			continue
		}
		host := key[len("gh-identity."):field]
		if id.Host != "" && id.Host != host {
			return nil, fmt.Errorf("identity %q contains multiple host definitions", name)
		}
		id.Host = host
		switch key[field+1:] {
		case "name":
			id.Name = value
		case "account":
			id.Account = value
		case "sshhost":
			id.SSHHost = value
		case "gitusername", "gitname":
			id.GitUserName = value
		case "gituseremail", "gitemail":
			id.GitUserEmail = value
		}
	}
	if id.Name != name || !safeName.MatchString(id.Host) || !safeName.MatchString(id.Account) {
		return nil, fmt.Errorf("invalid managed identity file %s", path)
	}
	return id, nil
}

// List returns all managed identities in name order.
func (s *Store) List() ([]Identity, error) {
	paths, err := filepath.Glob(filepath.Join(s.Dir, "*.gitconfig"))
	if err != nil {
		return nil, err
	}
	ids := make([]Identity, 0, len(paths))
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".gitconfig")
		id, err := s.Load(name)
		if err != nil {
			return nil, err
		}
		ids = append(ids, *id)
	}
	return ids, nil
}

// Bindings returns the persistent scope references, without resolving credentials.
func (s *Store) Bindings() ([]Binding, error) {
	for _, name := range []string{"bindings.json", "bindings.gitconfig"} {
		path := filepath.Join(s.Dir, name)
		if _, err := os.Stat(path); err == nil {
			return nil, fmt.Errorf("old prototype scope file %s is not supported; use a fresh identity-e2e run, or remove the old scope files and their global Git include before reapplying 'gh auth use'", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	path := s.authConfigPath()
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Binding{}, nil
	}
	if err != nil {
		return nil, err
	}
	out, err := gitOutput("", "config", "--no-includes", "--file", path, "--null", "--list")
	if err != nil {
		return nil, fmt.Errorf("read identity scopes from %s: %w", path, err)
	}
	bindings := []Binding{}
	sections := map[string]int{}
	seen := map[string]bool{}
	for entry := range strings.SplitSeq(out, "\x00") {
		key, value, ok := strings.Cut(entry, "\n")
		if !ok || !strings.HasPrefix(key, "gh-auth-binding.") {
			continue
		}
		dot := strings.LastIndexByte(key, '.')
		if dot <= len("gh-auth-binding.") || seen[key] || !validValue(value) {
			return nil, fmt.Errorf("invalid identity scope field %q in %s", key, path)
		}
		seen[key] = true
		section := key[:dot]
		index, ok := sections[section]
		if !ok {
			index = len(bindings)
			sections[section] = index
			bindings = append(bindings, Binding{})
		}
		b := &bindings[index]
		switch key[dot+1:] {
		case "identity":
			b.Identity = value
		case "host":
			b.Host = value
		case "directory":
			b.Directory = value
		case "repository":
			b.Repository = value
		case "owner":
			b.Owner = value
		default:
			return nil, fmt.Errorf("unsupported identity scope field %q in %s", key, path)
		}
	}
	for i, b := range bindings {
		scopes := 0
		for _, value := range []string{b.Directory, b.Repository, b.Owner} {
			if value != "" {
				scopes++
			}
		}
		if !safeName.MatchString(b.Identity) || !safeName.MatchString(b.Host) ||
			scopes != 1 ||
			(b.Directory != "" && (!filepath.IsAbs(b.Directory) || strings.ContainsAny(b.Directory, "*?[\\"))) ||
			(b.Repository != "" && (!filepath.IsAbs(b.Repository) || !validValue(b.Repository) || filepath.Base(b.Repository) != ".git" || strings.ContainsAny(b.Repository, "*?[\\"))) ||
			(b.Owner != "" && (!safeName.MatchString(b.Owner) || b.Owner != strings.ToLower(b.Owner))) {
			return nil, fmt.Errorf("invalid identity scope %d in %s", i, path)
		}
		for _, other := range bindings[:i] {
			if sameScope(b, other) {
				return nil, fmt.Errorf("duplicate identity scope %d in %s", i, path)
			}
		}
	}
	return bindings, nil
}

func (s *Store) authConfigPath() string {
	return filepath.Join(filepath.Dir(s.Dir), "auth.gitconfig")
}

func canonicalDirectory(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	path, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func normalizeBinding(b Binding) (Binding, error) {
	if b.Repository != "" {
		return b, errors.New("supply a checkout directory, not an internal repository scope")
	}
	if b.Owner != "" {
		if b.Directory != "" {
			return b, errors.New("owner choices cannot also specify a directory scope")
		}
		if !safeName.MatchString(b.Owner) {
			return b, errors.New("--owner must be a GitHub owner name")
		}
		b.Owner = strings.ToLower(b.Owner)
		return b, nil
	}
	dir, err := canonicalDirectory(b.Directory)
	if err != nil {
		return b, err
	}
	b.Directory = dir
	if _, err := os.Stat(dir); err != nil {
		return b, err
	}
	root, err := workingTreeRoot(dir)
	if err != nil {
		return b, err
	}
	if root != "" {
		if err := supportedCheckout(root); err != nil {
			return b, err
		}
		b.Repository, err = commonGitDirectory(dir)
		if err != nil {
			return b, err
		}
		b.Directory = ""
	} else {
		bare, err := gitOptional(dir, "rev-parse", "--is-bare-repository")
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 128 || !strings.Contains(err.Error(), "not a git repository") {
				return b, err
			}
		} else if bare == "true" {
			return b, errors.New("persistent identities in bare repositories are not supported; use a standalone checkout")
		}
	}
	path := b.Directory
	if b.Repository != "" {
		path = b.Repository
	}
	if !validValue(path) || strings.ContainsAny(path, "*?[\\") {
		return b, errors.New("binding paths must be valid UTF-8 without newlines, NUL, or Git glob metacharacters")
	}
	return b, nil
}

func sameScope(a, b Binding) bool {
	return a.Host == b.Host && a.Directory == b.Directory && a.Repository == b.Repository && a.Owner == b.Owner
}

// Bind replaces the binding for a scope and installs Git's managed configuration.
func (s *Store) Bind(b Binding, executable string) error {
	if runtime.GOOS == "windows" {
		return errors.New("persistent identities are not supported on Windows by this prototype")
	}
	if err := supportedGitVersion(); err != nil {
		return err
	}
	id, err := s.Load(b.Identity)
	if err != nil {
		return err
	}
	b.Host = id.Host
	contextDir := b.Directory
	if b.Owner == "" {
		contextDir, err = canonicalDirectory(contextDir)
		if err != nil {
			return err
		}
	}
	b, err = normalizeBinding(b)
	if err != nil {
		return err
	}
	if os.Getenv("GIT_SSH_COMMAND") != "" {
		return errors.New("GIT_SSH_COMMAND bypasses Git configuration, so persistent SSH identity routing cannot be installed; your SSH setup has not been changed")
	}
	contexts := []string{contextDir}
	if b.Repository != "" {
		contexts, err = familyDirectories(contextDir)
		if err != nil {
			return err
		}
	}
	for _, contextDir := range contexts {
		if b.Repository != "" {
			if err := supportedCheckout(contextDir); err != nil {
				return fmt.Errorf("worktree %s: %w", contextDir, err)
			}
		}
		if _, err := s.SSHCommand(contextDir, false); err != nil {
			return fmt.Errorf("context %s: %w", contextDir, err)
		}
		if err := s.supportedCredentialHelpers(contextDir, b.Host); err != nil {
			return err
		}
		if b.Owner == "" {
			executable, err = filepath.Abs(executable)
			if err != nil {
				return err
			}
			current, err := gitOptional(contextDir, "config", "--show-scope", "--get", "core.sshCommand")
			if err != nil {
				return err
			}
			scope, command, _ := strings.Cut(current, "\t")
			if current != "" && command != shellquote.Join(executable, "auth", "git-ssh") &&
				(scope == "local" || scope == "worktree" || scope == "command") {
				return fmt.Errorf("checkout %s: repository or command-level core.sshCommand would bypass gh's global SSH dispatcher; persistent identity routing is not supported for this setup, and your SSH configuration has not been changed", contextDir)
			}
			remotes, err := gitOptional(contextDir, "config", "--get-regexp", `^remote\..*\.url$`)
			if err != nil {
				return err
			}
			for line := range strings.SplitSeq(remotes, "\n") {
				_, raw, ok := strings.Cut(line, " ")
				if !ok {
					continue
				}
				u, err := git.ParseURL(raw)
				if err != nil {
					return fmt.Errorf("parse repository remote: %w", err)
				}
				if u.Scheme == "ssh" && strings.EqualFold(u.Host, id.Host) && id.SSHHost == "" {
					return fmt.Errorf("identity %q has no SSH route for this repository; edit it with --ssh-host <alias>", id.Name)
				}
			}
		}
	}
	bindings, err := s.Bindings()
	if err != nil {
		return err
	}
	bindings = slices.DeleteFunc(bindings, func(other Binding) bool { return sameScope(b, other) })
	bindings = append(bindings, b)
	if err := s.validateDirectoryAttribution(bindings, id); err != nil {
		return err
	}
	return s.writeBindings(bindings, executable)
}

func familyDirectories(dir string) ([]string, error) {
	out, err := gitOutput(dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var directories []string
	for field := range strings.SplitSeq(out, "\x00") {
		path, ok := strings.CutPrefix(field, "worktree ")
		if !ok {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			directories = append(directories, path)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if len(directories) == 0 {
		return nil, errors.New("Git reported no existing worktrees for this repository")
	}
	return directories, nil
}

func (s *Store) supportedCredentialHelpers(dir, host string) error {
	out, err := gitOutput(dir, "config", "--includes", "--show-origin", "--null", "--list")
	if err != nil {
		return err
	}
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	if out == "" {
		return nil
	}
	if len(fields)%2 != 0 {
		return errors.New("Git did not report credential helper origins")
	}
	var externalOrigins []string
	for i := 0; i < len(fields); i += 2 {
		origin := fields[i]
		if sameConfigFile(strings.TrimPrefix(origin, "file:"), s.authConfigPath()) {
			continue
		}
		key, value, _ := strings.Cut(fields[i+1], "\n")
		if !strings.HasPrefix(key, "credential.") || !strings.HasSuffix(key, ".helper") {
			continue
		}
		pathScoped := false
		if key != "credential.helper" {
			raw := strings.TrimSuffix(strings.TrimPrefix(key, "credential."), ".helper")
			u, err := git.ParseURL(raw)
			if err != nil || !strings.EqualFold(u.Hostname(), host) {
				continue
			}
			pathScoped = strings.Trim(u.Path, "/") != ""
		}
		if value == "" {
			if !pathScoped {
				externalOrigins = nil
			}
			continue
		}
		args, err := shellquote.Split(strings.TrimPrefix(value, "!"))
		if strings.HasPrefix(value, "!") && err == nil && len(args) == 3 && args[1] == "auth" && args[2] == "git-credential" {
			continue
		}
		if pathScoped {
			return fmt.Errorf("an existing HTTPS credential helper for a repository path from %s would be replaced; this prototype does not support that configuration", origin)
		}
		externalOrigins = append(externalOrigins, origin)
	}
	if len(externalOrigins) > 0 {
		return fmt.Errorf("an existing HTTPS credential helper from %s would be replaced; no identity choice was applied. If you intend gh to manage credentials for this host, first run 'gh auth setup-git --hostname %s'", externalOrigins[0], host)
	}
	return nil
}

func (s *Store) validateDirectoryAttribution(bindings []Binding, replacement *Identity) error {
	ids := map[string]*Identity{replacement.Name: replacement}
	for i, a := range bindings {
		for _, b := range bindings[:i] {
			if strings.EqualFold(a.Host, b.Host) || !scopesOverlap(a, b) {
				continue
			}
			for _, name := range []string{a.Identity, b.Identity} {
				if ids[name] == nil {
					id, err := s.Load(name)
					if err != nil {
						return err
					}
					ids[name] = id
				}
			}
			if ids[a.Identity].GitUserName != "" || ids[b.Identity].GitUserName != "" {
				return errors.New("Git attribution cannot be selected by host in overlapping repository, directory or owner choices; use separate directory trees, owner-only choices, or identities without Git name/email defaults")
			}
		}
	}
	return nil
}

func scopesOverlap(a, b Binding) bool {
	if a.Owner != "" && b.Owner != "" {
		return false
	}
	if a.Owner != "" || b.Owner != "" {
		return true
	}
	if a.Repository != "" && b.Repository != "" {
		return a.Repository == b.Repository
	}
	if a.Repository != "" {
		return under(filepath.Dir(a.Repository), b.Directory)
	}
	if b.Repository != "" {
		return under(filepath.Dir(b.Repository), a.Directory)
	}
	return under(a.Directory, b.Directory) || under(b.Directory, a.Directory)
}

// Unbind removes only the requested scope, revealing any broader binding.
func (s *Store) Unbind(b Binding, executable string) error {
	bindings, err := s.Bindings()
	if err != nil {
		return err
	}
	exact := false
	if b.Owner == "" {
		dir := b.Directory
		if dir == "" {
			dir = "."
		}
		dir, err = canonicalRemovalDirectory(dir)
		if err != nil {
			return err
		}
		if _, statErr := os.Stat(dir); statErr == nil {
			normalized, normErr := normalizeBinding(b)
			if normErr != nil {
				return normErr
			}
			if normalized.Repository != "" {
				for _, candidate := range bindings {
					if sameScope(normalized, candidate) {
						b, exact = candidate, true
						break
					}
				}
			}
		}
		for _, candidate := range bindings {
			if exact {
				break
			}
			if candidate.Host == b.Host && candidate.Directory == dir {
				b, exact = candidate, true
				break
			}
			if candidate.Host == b.Host && candidate.Repository != "" && filepath.Dir(candidate.Repository) == dir {
				b, exact = candidate, true
				break
			}
		}
	}
	if !exact {
		b, err = normalizeBinding(b)
		if err != nil {
			return err
		}
	}
	before := len(bindings)
	bindings = slices.DeleteFunc(bindings, func(other Binding) bool { return sameScope(b, other) })
	if len(bindings) == before {
		return errors.New("no identity binding exists for this scope")
	}
	return s.writeBindings(bindings, executable)
}

func canonicalRemovalDirectory(dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return canonical, nil
	}
	if !errors.Is(err, os.ErrNotExist) || filepath.Dir(absolute) == absolute {
		return "", err
	}
	parent, err := canonicalRemovalDirectory(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

func (s *Store) writeBindings(bindings []Binding, executable string) error {
	if len(bindings) == 0 {
		return s.clearBindings()
	}
	executable, err := filepath.Abs(executable)
	if err != nil {
		return err
	}
	managed := s.authConfigPath()
	sshCommand := shellquote.Join(executable, "auth", "git-ssh")
	for _, key := range []string{"core.sshCommand", "ssh.variant"} {
		if _, err := s.underlyingGitValue("", key); err != nil {
			return err
		}
	}
	sort.SliceStable(bindings, func(i, j int) bool {
		a, b := bindings[i], bindings[j]
		if scopeRank(a) != scopeRank(b) {
			return scopeRank(a) < scopeRank(b)
		}
		return len(a.Directory) < len(b.Directory)
	})
	var config strings.Builder
	fmt.Fprintln(&config, "# Managed by gh auth use. Do not edit.")
	for i, binding := range bindings {
		fmt.Fprintf(&config, "[gh-auth-binding %q]\n\tidentity = %s\n\thost = %s\n",
			strconv.Itoa(i), gitQuote(binding.Identity), gitQuote(binding.Host))
		if binding.Owner != "" {
			fmt.Fprintf(&config, "\towner = %s\n", gitQuote(binding.Owner))
		} else if binding.Repository != "" {
			fmt.Fprintf(&config, "\trepository = %s\n", gitQuote(binding.Repository))
		} else {
			fmt.Fprintf(&config, "\tdirectory = %s\n", gitQuote(binding.Directory))
		}
	}
	if len(bindings) > 0 {
		fmt.Fprintf(&config, "[core]\n\tsshCommand = %s\n[ssh]\n\tvariant = ssh\n", gitQuote(sshCommand))
	}
	hosts := map[string]bool{}
	for _, binding := range bindings {
		hosts[binding.Host] = true
	}
	for _, host := range sortedKeys(hosts) {
		helper := "!" + shellquote.Join(executable, "auth", "git-credential")
		fmt.Fprintf(&config, "[credential %s]\n\thelper =\n\thelper = %s\n\tuseHttpPath = true\n",
			gitQuote("https://"+host), gitQuote(helper))
	}
	for _, binding := range bindings {
		path := s.IdentityPath(binding.Identity)
		if binding.Owner != "" {
			host := ownerPattern(strings.ToLower(binding.Host))
			for _, prefix := range []string{
				"https://" + host + "/", "https://*@" + host + "/",
				"*@" + host + ":", "ssh://*@" + host + "/", "ssh://" + host + "/",
			} {
				condition := "hasconfig:remote.*.url:" + prefix + ownerPattern(binding.Owner) + "/**"
				fmt.Fprintf(&config, "[includeIf %s]\n\tpath = %s\n", gitQuote(condition), gitQuote(path))
			}
		} else if binding.Repository != "" {
			for _, pattern := range []string{filepath.ToSlash(binding.Repository), filepath.ToSlash(binding.Repository) + "/worktrees/*"} {
				fmt.Fprintf(&config, "[includeIf %s]\n\tpath = %s\n", gitQuote("gitdir:"+pattern), gitQuote(path))
			}
		} else {
			pattern := strings.TrimSuffix(filepath.ToSlash(binding.Directory), "/") + "/**/.git"
			fmt.Fprintf(&config, "[includeIf %s]\n\tpath = %s\n", gitQuote("gitdir:"+pattern), gitQuote(path))
			fmt.Fprintf(&config, "[includeIf %s]\n\tpath = %s\n", gitQuote("gitdir:"+pattern+"/worktrees/*"), gitQuote(path))
		}
	}
	oldConfig, readErr := os.ReadFile(managed)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	paths, err := gitOptional("", "config", "--global", "--get-all", "include.path")
	if err != nil {
		return err
	}
	if err := atomicWrite(managed, []byte(config.String())); err != nil {
		return err
	}
	addedInclude := !slices.Contains(strings.Split(paths, "\n"), managed)
	rollback := func(cause error, removeInclude bool) error {
		var restoreErr, includeErr error
		if readErr == nil {
			restoreErr = atomicWrite(managed, oldConfig)
		} else {
			restoreErr = os.Remove(managed)
		}
		if removeInclude {
			_, includeErr = gitOptional("", "config", "--global", "--fixed-value", "--unset-all", "include.path", managed)
		}
		return errors.Join(cause, restoreErr, includeErr)
	}
	if addedInclude {
		if _, err := gitOutput("", "config", "--global", "--add", "include.path", managed); err != nil {
			return rollback(err, false)
		}
	}
	if len(bindings) > 0 {
		current, err := gitOptional("", "config", "--global", "--includes", "--get", "core.sshCommand")
		if err != nil {
			return rollback(err, addedInclude)
		}
		if current != sshCommand {
			return rollback(errors.New("existing Git configuration takes precedence over gh's SSH dispatcher; the managed include must follow core.sshCommand, and no identity choice was applied"), addedInclude)
		}
	}
	return nil
}

func ownerPattern(owner string) string {
	var pattern strings.Builder
	for _, char := range owner {
		if char >= 'a' && char <= 'z' {
			fmt.Fprintf(&pattern, "[%c%c]", char, char-'a'+'A')
		} else {
			pattern.WriteRune(char)
		}
	}
	return pattern.String()
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func under(dir, parent string) bool {
	rel, err := filepath.Rel(parent, dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func workingTreeRoot(dir string) (string, error) {
	root, err := gitOptional(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 128 &&
			(strings.Contains(err.Error(), "not a git repository") || strings.Contains(err.Error(), "must be run in a work tree")) {
			return "", nil
		}
		return "", err
	}
	return root, nil
}

// Resolve selects a repository choice, then deepest directory tree, then owner.
// Outside a repository, directory-tree and explicit owner bindings still apply.
func (s *Store) Resolve(host, owner, dir string) (*Identity, error) {
	result, err := s.resolve(host, owner, dir)
	if err != nil {
		return nil, err
	}
	return result.Identity, nil
}

func (s *Store) resolve(host, owner, dir string) (*resolution, error) {
	bindings, err := s.Bindings()
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return &resolution{}, nil
	}
	return s.resolveBindings(host, owner, dir, bindings)
}

func (s *Store) resolveBindings(host, owner, dir string, bindings []Binding) (*resolution, error) {
	dir, err := canonicalDirectory(dir)
	if err != nil {
		return nil, err
	}
	root, err := workingTreeRoot(dir)
	if err != nil {
		return nil, err
	}
	// An explicit GIT_DIR can select a repository outside the caller's
	// directory. --show-toplevel alone can report the caller in that case.
	if os.Getenv("GIT_DIR") != "" {
		gitDir, err := gitOptional(dir, "rev-parse", "--absolute-git-dir")
		if err != nil {
			return nil, err
		}
		if filepath.Base(gitDir) == ".git" {
			dir, err = canonicalDirectory(filepath.Dir(gitDir))
			if err != nil {
				return nil, err
			}
			root = dir
		}
	}
	result := &resolution{}
	scopeDir := dir
	if root != "" {
		result.CommonGitDir, err = commonGitDirectory(dir)
		if err != nil {
			return nil, err
		}
		scopeDir = filepath.Dir(result.CommonGitDir)
	}
	if len(bindings) == 0 {
		return result, nil
	}
	for i := range bindings {
		b := &bindings[i]
		if !strings.EqualFold(b.Host, host) || b.Owner != "" {
			continue
		}
		if b.Repository != "" {
			if b.Repository == result.CommonGitDir {
				result.Matches = append(result.Matches, *b)
			}
			continue
		}

		if under(scopeDir, b.Directory) {
			result.Matches = append(result.Matches, *b)
		}
	}
	owners := map[string]bool{}
	if owner != "" {
		owners[strings.ToLower(owner)] = true
	} else if root != "" {
		urls, err := gitOptional(dir, "config", "--get-regexp", `^remote\..*\.url$`)
		if err != nil {
			return nil, err
		}
		for line := range strings.SplitSeq(urls, "\n") {
			_, raw, ok := strings.Cut(line, " ")
			if !ok {
				continue
			}
			u, err := git.ParseURL(raw)
			if err != nil || !strings.EqualFold(u.Host, host) {
				continue
			}
			owner, _, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
			owners[strings.ToLower(owner)] = true
		}
	}
	for i := range bindings {
		b := &bindings[i]
		if !strings.EqualFold(b.Host, host) || b.Owner == "" || !owners[b.Owner] {
			continue
		}
		result.Matches = append(result.Matches, *b)
	}
	sort.SliceStable(result.Matches, func(i, j int) bool {
		a, b := result.Matches[i], result.Matches[j]
		if scopeRank(a) != scopeRank(b) {
			return scopeRank(a) > scopeRank(b)
		}
		return len(a.Directory) > len(b.Directory)
	})
	if len(result.Matches) == 0 {
		return result, nil
	}
	if root != "" {
		if err := supportedCheckout(root); err != nil {
			return nil, err
		}
	}
	selected := result.Matches[0]
	if selected.Owner != "" {
		for _, other := range result.Matches[1:] {
			if other.Identity != selected.Identity {
				return nil, errors.New("multiple owners select different identities; bind this repository explicitly with 'gh auth use <identity>'")
			}
		}
	}
	result.Identity, err = s.Load(selected.Identity)
	return result, err
}

func supportedCheckout(root string) error {
	commonDir, err := commonGitDirectory(root)
	if err != nil {
		return err
	}
	if filepath.Base(commonDir) != ".git" {
		return errors.New("persistent identities in submodules or separate Git directories are not supported by this prototype; use a standalone clone or an explicit --account for gh commands")
	}
	modules := filepath.Join(root, ".gitmodules")
	if _, err := os.Stat(modules); err == nil {
		paths, err := gitOptional(root, "config", "--no-includes", "--file", modules, "--get-regexp", `^submodule\..*\.path$`)
		if err != nil {
			return err
		}
		if paths != "" {
			return errors.New("persistent identities in repositories that use submodules are not supported by this prototype; use a standalone repository without submodules")
		}

	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func commonGitDirectory(dir string) (string, error) {
	common, err := gitOptional(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return canonicalDirectory(common)
}

func scopeRank(b Binding) int {
	if b.Repository != "" {
		return 2
	}
	if b.Directory != "" {
		return 1
	}
	return 0
}

func supportedGitVersion() error {
	out, err := gitOptional("", "version")
	if err != nil {
		return err
	}
	version, ok := strings.CutPrefix(out, "git version ")
	parts := strings.Split(version, ".")
	if !ok || len(parts) < 2 {
		return errors.New("cannot determine Git version; identities require Git 2.36 or newer")
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil || major < 2 || (major == 2 && minor < 36) {
		return errors.New("identities require Git 2.36 or newer for owner-conditional configuration; no identity choice was applied")
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return string(out), nil
}

func gitOptional(dir string, args ...string) (string, error) {
	out, err := gitOutput(dir, args...)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", nil
	}
	return strings.TrimSuffix(out, "\n"), err
}

func atomicWrite(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".identity-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(contents); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := gitOutput("", "config", "--no-includes", "--file", file.Name(), "--list"); err != nil {
		return fmt.Errorf("refusing to install invalid Git configuration: %w", err)
	}
	return os.Rename(file.Name(), path)
}

type resolution struct {
	CommonGitDir string
	Identity     *Identity
	Matches      []Binding
}
