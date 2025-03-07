package update

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cli/cli/v2/pkg/cmd/extension"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/require"
)

func TestCheckForUpdate(t *testing.T) {
	scenarios := []struct {
		Name           string
		CurrentVersion string
		LatestVersion  string
		LatestURL      string
		ExpectsResult  bool
	}{
		{
			Name:           "latest is newer",
			CurrentVersion: "v0.0.1",
			LatestVersion:  "v1.0.0",
			LatestURL:      "https://www.spacejam.com/archive/spacejam/movie/jam.htm",
			ExpectsResult:  true,
		},
		{
			Name:           "current is prerelease",
			CurrentVersion: "v1.0.0-pre.1",
			LatestVersion:  "v1.0.0",
			LatestURL:      "https://www.spacejam.com/archive/spacejam/movie/jam.htm",
			ExpectsResult:  true,
		},
		{
			Name:           "current is built from source",
			CurrentVersion: "v1.2.3-123-gdeadbeef",
			LatestVersion:  "v1.2.3",
			LatestURL:      "https://www.spacejam.com/archive/spacejam/movie/jam.htm",
			ExpectsResult:  false,
		},
		{
			Name:           "current is built from source after a prerelease",
			CurrentVersion: "v1.2.3-rc.1-123-gdeadbeef",
			LatestVersion:  "v1.2.3",
			LatestURL:      "https://www.spacejam.com/archive/spacejam/movie/jam.htm",
			ExpectsResult:  true,
		},
		{
			Name:           "latest is newer than version build from source",
			CurrentVersion: "v1.2.3-123-gdeadbeef",
			LatestVersion:  "v1.2.4",
			LatestURL:      "https://www.spacejam.com/archive/spacejam/movie/jam.htm",
			ExpectsResult:  true,
		},
		{
			Name:           "latest is current",
			CurrentVersion: "v1.0.0",
			LatestVersion:  "v1.0.0",
			LatestURL:      "https://www.spacejam.com/archive/spacejam/movie/jam.htm",
			ExpectsResult:  false,
		},
		{
			Name:           "latest is older",
			CurrentVersion: "v0.10.0-pre.1",
			LatestVersion:  "v0.9.0",
			LatestURL:      "https://www.spacejam.com/archive/spacejam/movie/jam.htm",
			ExpectsResult:  false,
		},
	}

	for _, s := range scenarios {
		t.Run(s.Name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpClient := &http.Client{}
			httpmock.ReplaceTripper(httpClient, reg)

			reg.Register(
				httpmock.REST("GET", "repos/OWNER/REPO/releases/latest"),
				httpmock.StringResponse(fmt.Sprintf(`{
					"tag_name": "%s",
					"html_url": "%s"
				}`, s.LatestVersion, s.LatestURL)),
			)

			rel, err := CheckForUpdate(context.TODO(), httpClient, tempFilePath(), "OWNER/REPO", s.CurrentVersion)
			if err != nil {
				t.Fatal(err)
			}

			if len(reg.Requests) != 1 {
				t.Fatalf("expected 1 HTTP request, got %d", len(reg.Requests))
			}
			requestPath := reg.Requests[0].URL.Path
			if requestPath != "/repos/OWNER/REPO/releases/latest" {
				t.Errorf("HTTP path: %q", requestPath)
			}

			if !s.ExpectsResult {
				if rel != nil {
					t.Fatal("expected no new release")
				}
				return
			}
			if rel == nil {
				t.Fatal("expected to report new release")
			}

			if rel.Version != s.LatestVersion {
				t.Errorf("Version: %q", rel.Version)
			}
			if rel.URL != s.LatestURL {
				t.Errorf("URL: %q", rel.URL)
			}
		})
	}
}

func TestCheckForExtensionUpdate(t *testing.T) {
	now := time.Date(2024, 12, 17, 12, 0, 0, 0, time.UTC)
	previousTooSoon := now.Add(-23 * time.Hour).Add(-59 * time.Minute).Add(-59 * time.Second)
	previousOldEnough := now.Add(-24 * time.Hour)

	tests := []struct {
		name                string
		extCurrentVersion   string
		extLatestVersion    string
		extKind             extension.ExtensionKind
		extURL              string
		previousStateEntry  *StateEntry
		expectedStateEntry  *StateEntry
		expectedReleaseInfo *ReleaseInfo
		wantErr             bool
	}{
		{
			name:              "return latest release given git extension is out of date and no state entry",
			extCurrentVersion: "v0.1.0",
			extLatestVersion:  "v1.0.0",
			extKind:           extension.GitKind,
			extURL:            "http://example.com",
			expectedStateEntry: &StateEntry{
				CheckedForUpdateAt: now,
				LatestRelease: ReleaseInfo{
					Version: "v1.0.0",
					URL:     "http://example.com",
				},
			},
			expectedReleaseInfo: &ReleaseInfo{
				Version: "v1.0.0",
				URL:     "http://example.com",
			},
		},
		{
			name:              "return latest release given git extension is out of date and state entry is old enough",
			extCurrentVersion: "v0.1.0",
			extLatestVersion:  "v1.0.0",
			extKind:           extension.GitKind,
			extURL:            "http://example.com",
			previousStateEntry: &StateEntry{
				CheckedForUpdateAt: previousOldEnough,
				LatestRelease: ReleaseInfo{
					Version: "v0.1.0",
					URL:     "http://example.com",
				},
			},
			expectedStateEntry: &StateEntry{
				CheckedForUpdateAt: now,
				LatestRelease: ReleaseInfo{
					Version: "v1.0.0",
					URL:     "http://example.com",
				},
			},
			expectedReleaseInfo: &ReleaseInfo{
				Version: "v1.0.0",
				URL:     "http://example.com",
			},
		},
		{
			name:              "return nothing given git extension is out of date but state entry is too recent",
			extCurrentVersion: "v0.1.0",
			extLatestVersion:  "v1.0.0",
			extKind:           extension.GitKind,
			extURL:            "http://example.com",
			previousStateEntry: &StateEntry{
				CheckedForUpdateAt: previousTooSoon,
				LatestRelease: ReleaseInfo{
					Version: "v0.1.0",
					URL:     "http://example.com",
				},
			},
			expectedStateEntry: &StateEntry{
				CheckedForUpdateAt: previousTooSoon,
				LatestRelease: ReleaseInfo{
					Version: "v0.1.0",
					URL:     "http://example.com",
				},
			},
			expectedReleaseInfo: nil,
		},
		{
			name:              "return latest release given binary extension is out of date and no state entry",
			extCurrentVersion: "v0.1.0",
			extLatestVersion:  "v1.0.0",
			extKind:           extension.BinaryKind,
			extURL:            "http://example.com",
			expectedStateEntry: &StateEntry{
				CheckedForUpdateAt: now,
				LatestRelease: ReleaseInfo{
					Version: "v1.0.0",
					URL:     "http://example.com",
				},
			},
			expectedReleaseInfo: &ReleaseInfo{
				Version: "v1.0.0",
				URL:     "http://example.com",
			},
		},
		{
			name:              "return latest release given binary extension is out of date and state entry is old enough",
			extCurrentVersion: "v0.1.0",
			extLatestVersion:  "v1.0.0",
			extKind:           extension.BinaryKind,
			extURL:            "http://example.com",
			previousStateEntry: &StateEntry{
				CheckedForUpdateAt: previousOldEnough,
				LatestRelease: ReleaseInfo{
					Version: "v0.1.0",
					URL:     "http://example.com",
				},
			},
			expectedStateEntry: &StateEntry{
				CheckedForUpdateAt: now,
				LatestRelease: ReleaseInfo{
					Version: "v1.0.0",
					URL:     "http://example.com",
				},
			},
			expectedReleaseInfo: &ReleaseInfo{
				Version: "v1.0.0",
				URL:     "http://example.com",
			},
		},
		{
			name:              "return nothing given binary extension is out of date but state entry is too recent",
			extCurrentVersion: "v0.1.0",
			extLatestVersion:  "v1.0.0",
			extKind:           extension.BinaryKind,
			extURL:            "http://example.com",
			previousStateEntry: &StateEntry{
				CheckedForUpdateAt: previousTooSoon,
				LatestRelease: ReleaseInfo{
					Version: "v0.1.0",
					URL:     "http://example.com",
				},
			},
			expectedStateEntry: &StateEntry{
				CheckedForUpdateAt: previousTooSoon,
				LatestRelease: ReleaseInfo{
					Version: "v0.1.0",
					URL:     "http://example.com",
				},
			},
			expectedReleaseInfo: nil,
		},
		{
			name:                "return nothing given local extension with no state entry",
			extCurrentVersion:   "v0.1.0",
			extLatestVersion:    "v1.0.0",
			extKind:             extension.LocalKind,
			extURL:              "http://example.com",
			expectedStateEntry:  nil,
			expectedReleaseInfo: nil,
		},
		{
			name:              "return nothing given local extension despite state entry is old enough",
			extCurrentVersion: "v0.1.0",
			extLatestVersion:  "v1.0.0",
			extKind:           extension.LocalKind,
			extURL:            "http://example.com",
			previousStateEntry: &StateEntry{
				CheckedForUpdateAt: previousOldEnough,
				LatestRelease: ReleaseInfo{
					Version: "v0.1.0",
					URL:     "http://example.com",
				},
			},
			expectedStateEntry: &StateEntry{
				CheckedForUpdateAt: previousOldEnough,
				LatestRelease: ReleaseInfo{
					Version: "v0.1.0",
					URL:     "http://example.com",
				},
			},
			expectedReleaseInfo: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateDir := t.TempDir()
			em := &extensions.ExtensionManagerMock{
				UpdateDirFunc: func(name string) string {
					return filepath.Join(updateDir, name)
				},
			}

			ext := &extensions.ExtensionMock{
				NameFunc: func() string {
					return "extension-update-test"
				},
				CurrentVersionFunc: func() string {
					return tt.extCurrentVersion
				},
				LatestVersionFunc: func() string {
					return tt.extLatestVersion
				},
				IsLocalFunc: func() bool {
					return tt.extKind == extension.LocalKind
				},
				IsBinaryFunc: func() bool {
					return tt.extKind == extension.BinaryKind
				},
				URLFunc: func() string {
					return tt.extURL
				},
			}

			// UpdateAvailable is arguably code under test but moq does not support partial mocks so this is a little brittle.
			ext.UpdateAvailableFunc = func() bool {
				if ext.IsLocal() {
					panic("Local extensions do not get update notices")
				}

				// Actual extension versions should drive tests instead of managing UpdateAvailable separately.
				current := ext.CurrentVersion()
				latest := ext.LatestVersion()
				return current != "" && latest != "" && current != latest
			}

			// Setup previous state file for test as necessary
			stateFilePath := filepath.Join(em.UpdateDir(ext.Name()), "state.yml")
			if tt.previousStateEntry != nil {
				require.NoError(t, setStateEntry(stateFilePath, tt.previousStateEntry.CheckedForUpdateAt, tt.previousStateEntry.LatestRelease))
			}

			actual, err := CheckForExtensionUpdate(em, ext, now)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.Equal(t, tt.expectedReleaseInfo, actual)

			if tt.expectedStateEntry == nil {
				require.NoFileExists(t, stateFilePath)
			} else {
				stateEntry, err := getStateEntry(stateFilePath)
				require.NoError(t, err)
				require.Equal(t, tt.expectedStateEntry, stateEntry)
			}
		})
	}
}

func TestShouldCheckForUpdate(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected bool
	}{
		{
			name: "should not check when user has explicitly disable notifications",
			env: map[string]string{
				"GH_NO_UPDATE_NOTIFIER": "1",
			},
			expected: false,
		},
		{
			name: "should not check when user is in codespace",
			env: map[string]string{
				"CODESPACES": "1",
			},
			expected: false,
		},
		{
			name: "should not check when in GitHub Actions / Travis / Circle / Cirrus / GitLab / AppVeyor / CodeShip / dsari",
			env: map[string]string{
				"CI": "1",
			},
			expected: false,
		},
		{
			name: "should not check when in Jenkins / TeamCity",
			env: map[string]string{
				"BUILD_NUMBER": "1",
			},
			expected: false,
		},
		{
			name: "should not check when in TaskCluster / dsari",
			env: map[string]string{
				"RUN_ID": "1",
			},
			expected: false,
		},
		// TODO: Figure out how to refactor IsTerminal() to be testable
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			actual := ShouldCheckForUpdate()
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestShouldCheckForExtensionUpdate(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected bool
	}{
		{
			name: "should not check when user has explicitly disable notifications",
			env: map[string]string{
				"GH_NO_EXTENSION_UPDATE_NOTIFIER": "1",
			},
			expected: false,
		},
		{
			name: "should not check when user is in codespace",
			env: map[string]string{
				"CODESPACES": "1",
			},
			expected: false,
		},
		{
			name: "should not check when in GitHub Actions / Travis / Circle / Cirrus / GitLab / AppVeyor / CodeShip / dsari",
			env: map[string]string{
				"CI": "1",
			},
			expected: false,
		},
		{
			name: "should not check when in Jenkins / TeamCity",
			env: map[string]string{
				"BUILD_NUMBER": "1",
			},
			expected: false,
		},
		{
			name: "should not check when in TaskCluster / dsari",
			env: map[string]string{
				"RUN_ID": "1",
			},
			expected: false,
		},
		// TODO: Figure out how to refactor IsTerminal() to be testable
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			actual := ShouldCheckForExtensionUpdate()
			require.Equal(t, tt.expected, actual)
		})
	}
}

func tempFilePath() string {
	file, err := os.CreateTemp("", "")
	if err != nil {
		log.Fatal(err)
	}
	os.Remove(file.Name())
	return file.Name()
}
