package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/cli/cli/v2/internal/skills/discovery"
	"github.com/cli/cli/v2/internal/skills/frontmatter"
	"github.com/cli/cli/v2/internal/skills/lockfile"
	"github.com/cli/cli/v2/internal/skills/registry"
)

// maxConcurrency limits parallel API requests to avoid rate limiting.
const maxConcurrency = 5

// Options configures an installation.
type Options struct {
	Host       string // GitHub API hostname
	Owner      string
	Repo       string
	Ref        string // resolved ref name
	SHA        string // resolved commit SHA
	PinnedRef  string // user-supplied --pin value (empty if unpinned)
	Skills     []discovery.Skill
	AgentHost  *registry.AgentHost
	Scope      registry.Scope
	Dir        string // explicit target directory (overrides AgentHost+Scope)
	GitRoot    string // git repository root (for project scope)
	HomeDir    string // user home directory (for user scope)
	Client     *api.Client
	OnProgress func(done, total int) // called after each skill is installed
}

// Result tracks what was installed.
type Result struct {
	Installed []string
	Dir       string
	Warnings  []string
}

type skillResult struct {
	name string
	err  error
}

// Install fetches and writes skills to the target directory.
func Install(opts *Options) (*Result, error) {
	targetDir := opts.Dir
	if targetDir == "" {
		if opts.AgentHost == nil {
			return nil, fmt.Errorf("either Dir or AgentHost must be specified")
		}
		var err error
		targetDir, err = opts.AgentHost.InstallDir(opts.Scope, opts.GitRoot, opts.HomeDir)
		if err != nil {
			return nil, err
		}
	}

	if len(opts.Skills) == 1 {
		skill := opts.Skills[0]
		if opts.OnProgress != nil {
			opts.OnProgress(0, 1)
			defer opts.OnProgress(1, 1)
		}
		if err := installSkill(opts, skill, targetDir); err != nil {
			return nil, fmt.Errorf("failed to install skill %q: %w", skill.InstallName(), err)
		}
		var warnings []string
		if err := lockfile.RecordInstall(opts.Host, skill.InstallName(), opts.Owner, opts.Repo, skill.Path+"/SKILL.md", skill.TreeSHA, opts.PinnedRef); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not record install for %s: %v", skill.InstallName(), err))
		}
		return &Result{Installed: []string{skill.InstallName()}, Dir: targetDir, Warnings: warnings}, nil
	}

	total := len(opts.Skills)
	if opts.OnProgress != nil {
		opts.OnProgress(0, total)
	}

	type job struct {
		idx   int
		skill discovery.Skill
	}
	jobs := make(chan job)

	results := make([]skillResult, total)
	var wg sync.WaitGroup
	var done atomic.Int32

	workers := min(maxConcurrency, total)
	for range workers {
		wg.Go(func() {
			for j := range jobs {
				err := installSkill(opts, j.skill, targetDir)
				results[j.idx] = skillResult{name: j.skill.InstallName(), err: err}

				if opts.OnProgress != nil {
					opts.OnProgress(int(done.Add(1)), total)
				}
			}
		})
	}

	for i, s := range opts.Skills {
		jobs <- job{idx: i, skill: s}
	}
	close(jobs)
	wg.Wait()

	var installed []string
	var warnings []string
	var firstErr error
	for i, r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to install skill %q: %w", r.name, r.err)
			}
			continue
		}
		installed = append(installed, r.name)
		skill := opts.Skills[i]
		if err := lockfile.RecordInstall(opts.Host, skill.InstallName(), opts.Owner, opts.Repo, skill.Path+"/SKILL.md", skill.TreeSHA, opts.PinnedRef); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not record install for %s: %v", skill.InstallName(), err))
		}
	}

	if firstErr != nil {
		return &Result{Installed: installed, Dir: targetDir, Warnings: warnings}, firstErr
	}

	return &Result{Installed: installed, Dir: targetDir, Warnings: warnings}, nil
}

// LocalOptions configures a local directory installation.
type LocalOptions struct {
	SourceDir string
	Skills    []discovery.Skill
	AgentHost *registry.AgentHost
	Scope     registry.Scope
	Dir       string
	GitRoot   string
	HomeDir   string
}

// InstallLocal copies skills from a local directory to the target install location.
func InstallLocal(opts *LocalOptions) (*Result, error) {
	targetDir := opts.Dir
	if targetDir == "" {
		if opts.AgentHost == nil {
			return nil, fmt.Errorf("either Dir or AgentHost must be specified")
		}
		var err error
		targetDir, err = opts.AgentHost.InstallDir(opts.Scope, opts.GitRoot, opts.HomeDir)
		if err != nil {
			return nil, err
		}
	}

	var installed []string
	for _, skill := range opts.Skills {
		if err := installLocalSkill(opts.SourceDir, skill, targetDir); err != nil {
			return nil, fmt.Errorf("failed to install skill %q: %w", skill.InstallName(), err)
		}
		installed = append(installed, skill.InstallName())
	}

	return &Result{Installed: installed, Dir: targetDir}, nil
}

func installLocalSkill(sourceRoot string, skill discovery.Skill, baseDir string) error {
	// Use skill.Name (not InstallName) so skills are always installed flat.
	// Most agent clients only discover immediate subdirectories of their
	// skills folder and do not find skills nested under namespace directories.
	skillDir := filepath.Join(baseDir, skill.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("could not create directory %s: %w", skillDir, err)
	}

	srcDir := filepath.Join(sourceRoot, filepath.FromSlash(skill.Path))
	absSource, err := filepath.Abs(srcDir)
	if err != nil {
		return fmt.Errorf("could not resolve source path: %w", err)
	}

	safeSkillDir, err := safepaths.ParseAbsolute(skillDir)
	if err != nil {
		return fmt.Errorf("could not resolve target path: %w", err)
	}

	return filepath.WalkDir(srcDir, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}

		// Defensive: filepath.WalkDir cannot produce traversal paths, but we
		// guard against it in case the walk input is ever changed.
		safeDest, err := safeSkillDir.Join(relPath)
		if err != nil {
			var traversalErr safepaths.PathTraversalError
			if errors.As(err, &traversalErr) {
				return fmt.Errorf("blocked path traversal in %q", relPath)
			}
			return fmt.Errorf("could not resolve destination path: %w", err)
		}
		destPath := safeDest.String()

		if dir := filepath.Dir(destPath); dir != skillDir {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("could not create directory: %w", err)
			}
		}

		content, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("could not read %s: %w", p, err)
		}

		if filepath.Base(relPath) == "SKILL.md" {
			injected, injectErr := frontmatter.InjectLocalMetadata(string(content), absSource)
			if injectErr != nil {
				return fmt.Errorf("could not inject metadata: %w", injectErr)
			}
			content = []byte(injected)
		}

		return os.WriteFile(destPath, content, 0o644)
	})
}

func installSkill(opts *Options, skill discovery.Skill, baseDir string) error {
	// Use skill.Name (not InstallName) for a flat directory layout.
	skillDir := filepath.Join(baseDir, skill.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("could not create directory %s: %w", skillDir, err)
	}

	files, err := discovery.DiscoverSkillFiles(opts.Client, opts.Host, opts.Owner, opts.Repo, skill.TreeSHA, skill.Path)
	if err != nil {
		return fmt.Errorf("could not list skill files: %w", err)
	}

	safeSkillDir, err := safepaths.ParseAbsolute(skillDir)
	if err != nil {
		return fmt.Errorf("could not resolve skill directory path: %w", err)
	}

	for _, file := range files {
		content, err := discovery.FetchBlob(opts.Client, opts.Host, opts.Owner, opts.Repo, file.SHA)
		if err != nil {
			return fmt.Errorf("could not fetch %s: %w", file.Path, err)
		}

		relPath := strings.TrimPrefix(file.Path, skill.Path+"/")

		safeDest, err := safeSkillDir.Join(relPath)
		if err != nil {
			var traversalErr safepaths.PathTraversalError
			if errors.As(err, &traversalErr) {
				return fmt.Errorf("blocked path traversal in %q", relPath)
			}
			return fmt.Errorf("could not resolve destination path: %w", err)
		}
		destPath := safeDest.String()

		if dir := filepath.Dir(destPath); dir != skillDir {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("could not create directory: %w", err)
			}
		}

		if filepath.Base(relPath) == "SKILL.md" {
			content, err = frontmatter.InjectGitHubMetadata(content, opts.Host, opts.Owner, opts.Repo, opts.Ref, skill.TreeSHA, opts.PinnedRef, skill.Path)
			if err != nil {
				return fmt.Errorf("could not inject metadata: %w", err)
			}
		}

		if err := os.WriteFile(destPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("could not write %s: %w", destPath, err)
		}
	}

	return nil
}

// ResolveGitRoot returns the git repository root using the provided client,
// falling back to the current working directory on error.
func ResolveGitRoot(gc *git.Client) string {
	if gc != nil && gc.RepoDir != "" {
		return gc.RepoDir
	}
	if gc != nil {
		if root, err := gc.ToplevelDir(context.Background()); err == nil {
			return root
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

// ResolveHomeDir returns the user's home directory, or "" on error.
func ResolveHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
