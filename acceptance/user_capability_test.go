package acceptance_test

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var fixtureRepositoryEnvironmentVariable = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
var directRepositoryCreation = regexp.MustCompile(`^(?:\[[^]]+\] )*(?:! )?exec gh repo create(?: |$)`)

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
		return fmt.Errorf("%s: script must contain exactly one fixture-repo declaration", file)
	}

	declaration := declarations[0]
	switch {
	case len(declaration) == 2 && declaration[1] == "none":
		return nil
	case len(declaration) == 3 && (declaration[1] == "shared" || declaration[1] == "isolated"):
		if !fixtureRepositoryEnvironmentVariable.MatchString(declaration[2]) {
			return fmt.Errorf("%s: fixture repository environment variable must match %s", file, fixtureRepositoryEnvironmentVariable)
		}
		if hasDirectRepositoryCreation {
			return fmt.Errorf("%s: scripts using a managed fixture repository must not run gh repo create", file)
		}
		return nil
	default:
		return fmt.Errorf("%s: fixture-repo declaration must be 'fixture-repo shared ENV_VAR', 'fixture-repo isolated ENV_VAR', or 'fixture-repo none'", file)
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
			wantErr: "exactly one",
		},
		{
			name:    "multiple",
			content: "fixture-repo shared REPO\nfixture-repo isolated OTHER_REPO\n",
			wantErr: "exactly one",
		},
		{
			name:    "invalid mode",
			content: "fixture-repo pristine REPO\n",
			wantErr: "declaration must be",
		},
		{
			name:    "invalid environment variable",
			content: "fixture-repo shared repo\n",
			wantErr: "environment variable",
		},
		{
			name:    "managed fixture creates repository",
			content: "fixture-repo isolated REPO\nexec gh repo create $ORG/example --private\n",
			wantErr: "must not run gh repo create",
		},
		{
			name:    "managed fixture conditionally creates repository",
			content: "fixture-repo shared REPO\n[windows] ! exec gh repo create $ORG/example --private\n",
			wantErr: "must not run gh repo create",
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
