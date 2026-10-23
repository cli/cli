package acceptance_test

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fixtureRepositoryEnvironmentVariable = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
var directRepositoryCreation = regexp.MustCompile(`^(?:\[[^]]+\] )*(?:! )?exec gh repo create(?: |$)`)

const fixtureRepositoryDeclarationHelp = `choose one:
  fixture-repo shared REPO
  fixture-repo isolated REPO
  fixture-repo none

Use shared when concurrent scripts can safely reuse the repository, isolated
when the script needs clean or repository-global state, and none when the
script creates its own repositories or does not need one.
See acceptance/README.md#script-metadata for details.`

func tokenHasUserCapability(token string) (bool, error) {
	switch {
	case strings.HasPrefix(token, "ghp_"),
		strings.HasPrefix(token, "gho_"),
		strings.HasPrefix(token, "github_pat_"),
		strings.HasPrefix(token, "ghu_"):
		return true, nil
	case strings.HasPrefix(token, "ghs_"):
		return false, nil
	default:
		return false, fmt.Errorf("GH_ACCEPTANCE_TOKEN has an unsupported token prefix")
	}
}

func requiresUserCapabilityForScript(file string) (bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return false, fmt.Errorf("%s: first line must be '# requires-user-capability: true' or '# requires-user-capability: false'", file)
	}

	var requiresUserCapability bool
	switch strings.TrimSuffix(scanner.Text(), "\r") {
	case "# requires-user-capability: true":
		requiresUserCapability = true
	case "# requires-user-capability: false":
		requiresUserCapability = false
	default:
		return false, fmt.Errorf("%s: first line must be '# requires-user-capability: true' or '# requires-user-capability: false'", file)
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.HasPrefix(line, "-- ") && strings.HasSuffix(line, " --") {
			break
		}
		if strings.HasPrefix(line, "# requires-user-capability:") {
			return false, fmt.Errorf("%s: script must contain exactly one requires-user-capability declaration", file)
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}

	return requiresUserCapability, nil
}

func validateFixtureRepositoryDeclaration(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	var declarations [][]string
	hasDirectRepositoryCreation := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.HasPrefix(line, "-- ") && strings.HasSuffix(line, " --") {
			break
		}
		if strings.HasPrefix(line, "fixture-repo ") {
			declarations = append(declarations, strings.Fields(line))
		}
		if directRepositoryCreation.MatchString(line) {
			hasDirectRepositoryCreation = true
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if len(declarations) != 1 {
		return fmt.Errorf("%s: script must contain exactly one fixture-repo declaration; %s", file, fixtureRepositoryDeclarationHelp)
	}

	declaration := declarations[0]
	switch {
	case len(declaration) == 2 && declaration[1] == "none":
		return nil
	case len(declaration) == 3 && (declaration[1] == "shared" || declaration[1] == "isolated"):
		if !fixtureRepositoryEnvironmentVariable.MatchString(declaration[2]) {
			return fmt.Errorf("%s: fixture repository environment variable must match %s; for example:\n  fixture-repo shared REPO", file, fixtureRepositoryEnvironmentVariable)
		}
		if hasDirectRepositoryCreation {
			return fmt.Errorf("%s: scripts using a managed fixture repository must not run gh repo create; either remove gh repo create or use:\n  fixture-repo none", file)
		}
		return nil
	default:
		return fmt.Errorf("%s: invalid fixture-repo declaration; %s", file, fixtureRepositoryDeclarationHelp)
	}
}

func TestTokenHasUserCapability(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{token: "ghp_token", want: true},
		{token: "gho_token", want: true},
		{token: "github_pat_token", want: true},
		{token: "ghu_token", want: true},
		{token: "ghs_token", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.token[:4], func(t *testing.T) {
			got, err := tokenHasUserCapability(tt.token)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := tokenHasUserCapability("unsupported")
	assert.EqualError(t, err, "GH_ACCEPTANCE_TOKEN has an unsupported token prefix")
}

func TestAcceptanceScriptsDeclareUserCapabilityRequirement(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*", "*.txtar"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			_, err := requiresUserCapabilityForScript(file)
			require.NoError(t, err)
		})
	}
}

func TestAcceptanceScriptsDeclareFixtureRepository(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*", "*.txtar"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			require.NoError(t, validateFixtureRepositoryDeclaration(file))
		})
	}
}

func TestRequiresUserCapabilityForScript(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
		wantErr string
	}{
		{
			name:    "requires user capability",
			content: "# requires-user-capability: true\n",
			want:    true,
		},
		{
			name:    "does not require user capability",
			content: "# requires-user-capability: false\n",
		},
		{
			name:    "supports CRLF",
			content: "# requires-user-capability: true\r\n",
			want:    true,
		},
		{
			name:    "missing",
			content: "",
			wantErr: "first line must be",
		},
		{
			name:    "not first",
			content: "# explanation\n# requires-user-capability: true\n",
			wantErr: "first line must be",
		},
		{
			name:    "invalid value",
			content: "# requires-user-capability: unknown\n",
			wantErr: "first line must be",
		},
		{
			name:    "multiple",
			content: "# requires-user-capability: false\n# requires-user-capability: true\n",
			wantErr: "script must contain exactly one requires-user-capability declaration",
		},
		{
			name: "ignores archive contents",
			content: "# requires-user-capability: false\n\n" +
				"-- script.sh --\n" +
				"# requires-user-capability: true\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "script.txtar")
			require.NoError(t, os.WriteFile(file, []byte(tt.content), 0o600))

			got, err := requiresUserCapabilityForScript(file)
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestValidateFixtureRepositoryDeclaration(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "shared",
			content: "fixture-repo shared REPO\n",
		},
		{
			name:    "isolated",
			content: "fixture-repo isolated REPO\n",
		},
		{
			name:    "none",
			content: "fixture-repo none\n",
		},
		{
			name:    "missing",
			content: "",
			wantErr: "choose one:\n  fixture-repo shared REPO\n  fixture-repo isolated REPO\n  fixture-repo none",
		},
		{
			name:    "multiple",
			content: "fixture-repo shared REPO\nfixture-repo isolated OTHER_REPO\n",
			wantErr: "choose one:\n  fixture-repo shared REPO\n  fixture-repo isolated REPO\n  fixture-repo none",
		},
		{
			name:    "invalid mode",
			content: "fixture-repo pristine REPO\n",
			wantErr: "choose one:\n  fixture-repo shared REPO\n  fixture-repo isolated REPO\n  fixture-repo none",
		},
		{
			name:    "invalid environment variable",
			content: "fixture-repo shared repo\n",
			wantErr: "for example:\n  fixture-repo shared REPO",
		},
		{
			name:    "managed fixture creates repository",
			content: "fixture-repo isolated REPO\nexec gh repo create $ORG/example --private\n",
			wantErr: "either remove gh repo create or use:\n  fixture-repo none",
		},
		{
			name:    "managed fixture conditionally creates repository",
			content: "fixture-repo shared REPO\n[windows] ! exec gh repo create $ORG/example --private\n",
			wantErr: "either remove gh repo create or use:\n  fixture-repo none",
		},
		{
			name: "ignores archive contents",
			content: "fixture-repo none\n\n" +
				"-- script.sh --\n" +
				"fixture-repo shared REPO\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "script.txtar")
			require.NoError(t, os.WriteFile(file, []byte(tt.content), 0o600))

			err := validateFixtureRepositoryDeclaration(file)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}
