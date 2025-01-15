package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/hashicorp/go-version"
	"github.com/mattn/go-isatty"
	"gopkg.in/yaml.v3"
)

var gitDescribeSuffixRE = regexp.MustCompile(`\d+-\d+-g[a-f0-9]{8}$`)

// ReleaseInfo stores information about a release
type ReleaseInfo struct {
	Version     string    `json:"tag_name"`
	URL         string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

type StateEntry struct {
	CheckedForUpdateAt time.Time   `yaml:"checked_for_update_at"`
	LatestRelease      ReleaseInfo `yaml:"latest_release"`
}

// ShouldCheckForExtensionUpdate decides whether we check for updates for GitHub CLI extensions based on user preferences and current execution context.
// During cli/cli#9934, this logic was split out from ShouldCheckForUpdate() because we envisioned it going in a different direction.
func ShouldCheckForExtensionUpdate() bool {
	if os.Getenv("GH_NO_EXTENSION_UPDATE_NOTIFIER") != "" {
		return false
	}
	if os.Getenv("CODESPACES") != "" {
		return false
	}
	return !IsCI() && IsTerminal(os.Stdout) && IsTerminal(os.Stderr)
}

// CheckForExtensionUpdate checks whether an update exists for a specific extension based on extension type and recency of last check within past 24 hours.
func CheckForExtensionUpdate(em extensions.ExtensionManager, ext extensions.Extension, now time.Time) (*ReleaseInfo, error) {
	// local extensions cannot have updates, so avoid work that ultimately returns nothing.
	if ext.IsLocal() {
		return nil, nil
	}

	stateFilePath := filepath.Join(em.UpdateDir(ext.Name()), "state.yml")
	stateEntry, _ := getStateEntry(stateFilePath)
	if stateEntry != nil && now.Sub(stateEntry.CheckedForUpdateAt).Hours() < 24 {
		return nil, nil
	}

	releaseInfo := &ReleaseInfo{
		Version: ext.LatestVersion(),
		URL:     ext.URL(),
	}

	err := setStateEntry(stateFilePath, now, *releaseInfo)
	if err != nil {
		return nil, err
	}

	if ext.UpdateAvailable() {
		return releaseInfo, nil
	}

	return nil, nil
}

// ShouldCheckForUpdate decides whether we check for updates for the GitHub CLI based on user preferences and current execution context.
func ShouldCheckForUpdate() bool {
	if os.Getenv("GH_NO_UPDATE_NOTIFIER") != "" {
		return false
	}
	if os.Getenv("CODESPACES") != "" {
		return false
	}
	return !IsCI() && IsTerminal(os.Stdout) && IsTerminal(os.Stderr)
}

// CheckForUpdate checks whether an update exists for the GitHub CLI based on recency of last check within past 24 hours.
func CheckForUpdate(ctx context.Context, client *http.Client, stateFilePath, repo, currentVersion string) (*ReleaseInfo, error) {
	stateEntry, _ := getStateEntry(stateFilePath)
	if stateEntry != nil && time.Since(stateEntry.CheckedForUpdateAt).Hours() < 24 {
		return nil, nil
	}

	releaseInfo, err := getLatestReleaseInfo(ctx, client, repo)
	if err != nil {
		return nil, err
	}

	err = setStateEntry(stateFilePath, time.Now(), *releaseInfo)
	if err != nil {
		return nil, err
	}

	if versionGreaterThan(releaseInfo.Version, currentVersion) {
		return releaseInfo, nil
	}

	return nil, nil
}

func getLatestReleaseInfo(ctx context.Context, client *http.Client, repo string) (*ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected HTTP %d", res.StatusCode)
	}
	dec := json.NewDecoder(res.Body)
	var latestRelease ReleaseInfo
	if err := dec.Decode(&latestRelease); err != nil {
		return nil, err
	}
	return &latestRelease, nil
}

func getStateEntry(stateFilePath string) (*StateEntry, error) {
	content, err := os.ReadFile(stateFilePath)
	if err != nil {
		return nil, err
	}

	var stateEntry StateEntry
	err = yaml.Unmarshal(content, &stateEntry)
	if err != nil {
		return nil, err
	}

	return &stateEntry, nil
}

func setStateEntry(stateFilePath string, t time.Time, r ReleaseInfo) error {
	data := StateEntry{CheckedForUpdateAt: t, LatestRelease: r}
	content, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(stateFilePath), 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(stateFilePath, content, 0600)
	return err
}

func versionGreaterThan(v, w string) bool {
	w = gitDescribeSuffixRE.ReplaceAllStringFunc(w, func(m string) string {
		idx := strings.IndexRune(m, '-')
		n, _ := strconv.Atoi(m[0:idx])
		return fmt.Sprintf("%d-pre.0", n+1)
	})

	vv, ve := version.NewVersion(v)
	vw, we := version.NewVersion(w)

	return ve == nil && we == nil && vv.GreaterThan(vw)
}

// IsTerminal determines if a file descriptor is an interactive terminal / TTY.
func IsTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// IsCI determines if the current execution context is within a known CI/CD system.
// This is based on https://github.com/watson/ci-info/blob/HEAD/index.js.
func IsCI() bool {
	return os.Getenv("CI") != "" || // GitHub Actions, Travis CI, CircleCI, Cirrus CI, GitLab CI, AppVeyor, CodeShip, dsari
		os.Getenv("BUILD_NUMBER") != "" || // Jenkins, TeamCity
		os.Getenv("RUN_ID") != "" // TaskCluster, dsari
}
