package extension

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdExtension(t *testing.T) {
	tempDir := t.TempDir()
	oldWd, _ := os.Getwd()
	assert.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	tests := []struct {
		name         string
		args         []string
		managerStubs func(em *extensions.ExtensionManagerMock) func(*testing.T)
		isTTY        bool
		wantErr      bool
		errMsg       string
		wantStdout   string
		wantStderr   string
	}{
		{
			name: "install an extension",
			args: []string{"install", "owner/gh-some-ext"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.ListFunc = func(bool) []extensions.Extension {
					return []extensions.Extension{}
				}
				em.InstallFunc = func(_ ghrepo.Interface) error {
					return nil
				}
				return func(t *testing.T) {
					installCalls := em.InstallCalls()
					assert.Equal(t, 1, len(installCalls))
					assert.Equal(t, "gh-some-ext", installCalls[0].InterfaceMoqParam.RepoName())
					listCalls := em.ListCalls()
					assert.Equal(t, 1, len(listCalls))
				}
			},
		},
		{
			name: "install an extension with same name as existing extension",
			args: []string{"install", "owner/gh-existing-ext"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.ListFunc = func(bool) []extensions.Extension {
					e := &Extension{path: "owner2/gh-existing-ext"}
					return []extensions.Extension{e}
				}
				return func(t *testing.T) {
					calls := em.ListCalls()
					assert.Equal(t, 1, len(calls))
				}
			},
			wantErr: true,
			errMsg:  "there is already an installed extension that provides the \"existing-ext\" command",
		},
		{
			name: "install local extension",
			args: []string{"install", "."},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.InstallLocalFunc = func(dir string) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.InstallLocalCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, tempDir, normalizeDir(calls[0].Dir))
				}
			},
		},
		{
			name:    "upgrade error",
			args:    []string{"upgrade"},
			wantErr: true,
			errMsg:  "must specify an extension to upgrade",
		},
		{
			name: "upgrade an extension",
			args: []string{"upgrade", "hello"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.UpgradeFunc = func(name string, force bool, out, errOut io.Writer) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.UpgradeCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "hello", calls[0].Name)
				}
			},
		},
		{
			name: "upgrade an extension gh-prefix",
			args: []string{"upgrade", "gh-hello"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.UpgradeFunc = func(name string, force bool, out, errOut io.Writer) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.UpgradeCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "hello", calls[0].Name)
				}
			},
		},
		{
			name: "upgrade an extension full name",
			args: []string{"upgrade", "monalisa/gh-hello"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.UpgradeFunc = func(name string, force bool, out, errOut io.Writer) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.UpgradeCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "hello", calls[0].Name)
				}
			},
		},
		{
			name: "upgrade all",
			args: []string{"upgrade", "--all"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.UpgradeFunc = func(name string, force bool, out, errOut io.Writer) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.UpgradeCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "", calls[0].Name)
				}
			},
		},
		{
			name: "remove extension tty",
			args: []string{"remove", "hello"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.RemoveFunc = func(name string) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.RemoveCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "hello", calls[0].Name)
				}
			},
			isTTY:      true,
			wantStdout: "✓ Removed extension hello\n",
		},
		{
			name: "remove extension nontty",
			args: []string{"remove", "hello"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.RemoveFunc = func(name string) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.RemoveCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "hello", calls[0].Name)
				}
			},
			isTTY:      false,
			wantStdout: "",
		},
		{
			name: "remove extension gh-prefix",
			args: []string{"remove", "gh-hello"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.RemoveFunc = func(name string) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.RemoveCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "hello", calls[0].Name)
				}
			},
			isTTY:      false,
			wantStdout: "",
		},
		{
			name: "remove extension full name",
			args: []string{"remove", "monalisa/gh-hello"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.RemoveFunc = func(name string) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.RemoveCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "hello", calls[0].Name)
				}
			},
			isTTY:      false,
			wantStdout: "",
		},
		{
			name: "list extensions",
			args: []string{"list"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.ListFunc = func(bool) []extensions.Extension {
					ex1 := &Extension{path: "cli/gh-test", url: "https://github.com/cli/gh-test", updateAvailable: false}
					ex2 := &Extension{path: "cli/gh-test2", url: "https://github.com/cli/gh-test2", updateAvailable: true}
					return []extensions.Extension{ex1, ex2}
				}
				return func(t *testing.T) {
					assert.Equal(t, 1, len(em.ListCalls()))
				}
			},
			wantStdout: "gh test\tcli/gh-test\t\ngh test2\tcli/gh-test2\tUpgrade available\n",
		},
		{
			name: "create extension tty",
			args: []string{"create", "test"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.CreateFunc = func(name string) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.CreateCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "gh-test", calls[0].Name)
				}
			},
			isTTY: true,
			wantStdout: heredoc.Doc(`
				✓ Created directory gh-test
				✓ Initialized git repository
				✓ Set up extension scaffolding

				gh-test is ready for development

				Install locally with: cd gh-test && gh extension install .

				Publish to GitHub with: gh repo create gh-test

				For more information on writing extensions:
				https://docs.github.com/github-cli/github-cli/creating-github-cli-extensions
			`),
		},
		{
			name: "create extension notty",
			args: []string{"create", "gh-test"},
			managerStubs: func(em *extensions.ExtensionManagerMock) func(*testing.T) {
				em.CreateFunc = func(name string) error {
					return nil
				}
				return func(t *testing.T) {
					calls := em.CreateCalls()
					assert.Equal(t, 1, len(calls))
					assert.Equal(t, "gh-test", calls[0].Name)
				}
			},
			isTTY:      false,
			wantStdout: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			var assertFunc func(*testing.T)
			em := &extensions.ExtensionManagerMock{}
			if tt.managerStubs != nil {
				assertFunc = tt.managerStubs(em)
			}

			reg := httpmock.Registry{}
			defer reg.Verify(t)
			client := http.Client{Transport: &reg}

			f := cmdutil.Factory{
				Config: func() (config.Config, error) {
					return config.NewBlankConfig(), nil
				},
				IOStreams:        ios,
				ExtensionManager: em,
				HttpClient: func() (*http.Client, error) {
					return &client, nil
				},
			}

			cmd := NewCmdExtension(&f)
			cmd.SetArgs(tt.args)
			cmd.SetOut(ioutil.Discard)
			cmd.SetErr(ioutil.Discard)

			_, err := cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
			} else {
				assert.NoError(t, err)
			}

			if assertFunc != nil {
				assertFunc(t)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func normalizeDir(d string) string {
	return strings.TrimPrefix(d, "/private")
}

func Test_checkValidExtension(t *testing.T) {
	rootCmd := &cobra.Command{}
	rootCmd.AddCommand(&cobra.Command{Use: "help"})
	rootCmd.AddCommand(&cobra.Command{Use: "auth"})

	m := &extensions.ExtensionManagerMock{
		ListFunc: func(bool) []extensions.Extension {
			return []extensions.Extension{
				&extensions.ExtensionMock{
					NameFunc: func() string { return "screensaver" },
				},
				&extensions.ExtensionMock{
					NameFunc: func() string { return "triage" },
				},
			}
		},
	}

	type args struct {
		rootCmd *cobra.Command
		manager extensions.ExtensionManager
		extName string
	}
	tests := []struct {
		name      string
		args      args
		wantError string
	}{
		{
			name: "valid extension",
			args: args{
				rootCmd: rootCmd,
				manager: m,
				extName: "gh-hello",
			},
		},
		{
			name: "invalid extension name",
			args: args{
				rootCmd: rootCmd,
				manager: m,
				extName: "gherkins",
			},
			wantError: "extension repository name must start with `gh-`",
		},
		{
			name: "clashes with built-in command",
			args: args{
				rootCmd: rootCmd,
				manager: m,
				extName: "gh-auth",
			},
			wantError: "\"auth\" matches the name of a built-in command",
		},
		{
			name: "clashes with an installed extension",
			args: args{
				rootCmd: rootCmd,
				manager: m,
				extName: "gh-triage",
			},
			wantError: "there is already an installed extension that provides the \"triage\" command",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkValidExtension(tt.args.rootCmd, tt.args.manager, tt.args.extName)
			if tt.wantError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantError)
			}
		})
	}
}
