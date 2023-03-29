package create

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name      string
		tty       bool
		cli       string
		wantsErr  bool
		errMsg    string
		wantsOpts CreateOptions
	}{
		{
			name:      "no args tty",
			tty:       true,
			cli:       "",
			wantsOpts: CreateOptions{Interactive: true},
		},
		{
			name:     "no args no-tty",
			tty:      false,
			cli:      "",
			wantsErr: true,
			errMsg:   "at least one argument required in non-interactive mode",
		},
		{
			name: "new repo from remote",
			cli:  "NEWREPO --public --clone",
			wantsOpts: CreateOptions{
				Name:   "NEWREPO",
				Public: true,
				Clone:  true},
		},
		{
			name:     "no visibility",
			tty:      true,
			cli:      "NEWREPO",
			wantsErr: true,
			errMsg:   "`--public`, `--private`, or `--internal` required when not running interactively",
		},
		{
			name:     "multiple visibility",
			tty:      true,
			cli:      "NEWREPO --public --private",
			wantsErr: true,
			errMsg:   "expected exactly one of `--public`, `--private`, or `--internal`",
		},
		{
			name: "new remote from local",
			cli:  "--source=/path/to/repo --private",
			wantsOpts: CreateOptions{
				Private: true,
				Source:  "/path/to/repo"},
		},
		{
			name: "new remote from local with remote",
			cli:  "--source=/path/to/repo --public --remote upstream",
			wantsOpts: CreateOptions{
				Public: true,
				Source: "/path/to/repo",
				Remote: "upstream",
			},
		},
		{
			name: "new remote from local with push",
			cli:  "--source=/path/to/repo --push --public",
			wantsOpts: CreateOptions{
				Public: true,
				Source: "/path/to/repo",
				Push:   true,
			},
		},
		{
			name: "new remote from local without visibility",
			cli:  "--source=/path/to/repo --push",
			wantsOpts: CreateOptions{
				Source: "/path/to/repo",
				Push:   true,
			},
			wantsErr: true,
			errMsg:   "`--public`, `--private`, or `--internal` required when not running interactively",
		},
		{
			name:     "source with template",
			cli:      "--source=/path/to/repo --private --template mytemplate",
			wantsErr: true,
			errMsg:   "the `--source` option is not supported with `--clone`, `--template`, `--license`, or `--gitignore`",
		},
		{
			name:     "include all branches without template",
			cli:      "--source=/path/to/repo --private --include-all-branches",
			wantsErr: true,
			errMsg:   "the `--include-all-branches` option is only supported when using `--template`",
		},
		{
			name: "new remote from template with include all branches",
			cli:  "template-repo --template https://github.com/OWNER/REPO --public --include-all-branches",
			wantsOpts: CreateOptions{
				Name:               "template-repo",
				Public:             true,
				Template:           "https://github.com/OWNER/REPO",
				IncludeAllBranches: true,
			},
		},
		{
			name:     "template with .gitignore",
			cli:      "template-repo --template mytemplate --gitignore ../.gitignore --public",
			wantsErr: true,
			errMsg:   ".gitignore and license templates are not added when template is provided",
		},
		{
			name:     "template with license",
			cli:      "template-repo --template mytemplate --license ../.license --public",
			wantsErr: true,
			errMsg:   ".gitignore and license templates are not added when template is provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var opts *CreateOptions
			cmd := NewCmdCreate(f, func(o *CreateOptions) error {
				opts = o
				return nil
			})

			// TODO STUPID HACK
			// cobra aggressively adds help to all commands. since we're not running through the root command
			// (which manages help when running for real) and since create has a '-h' flag (for homepage),
			// cobra blows up when it tried to add a help flag and -h is already in use. This hack adds a
			// dummy help flag with a random shorthand to get around this.
			cmd.Flags().BoolP("help", "x", false, "")

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantsOpts.Interactive, opts.Interactive)
			assert.Equal(t, tt.wantsOpts.Source, opts.Source)
			assert.Equal(t, tt.wantsOpts.Name, opts.Name)
			assert.Equal(t, tt.wantsOpts.Public, opts.Public)
			assert.Equal(t, tt.wantsOpts.Internal, opts.Internal)
			assert.Equal(t, tt.wantsOpts.Private, opts.Private)
			assert.Equal(t, tt.wantsOpts.Clone, opts.Clone)
		})
	}
}

func Test_createRun(t *testing.T) {
	tests := []struct {
		name        string
		tty         bool
		opts        *CreateOptions
		httpStubs   func(*httpmock.Registry)
		promptStubs func(*prompter.PrompterMock)
		execStubs   func(*run.CommandStubber)
		wantStdout  string
		wantErr     bool
		errMsg      string
	}{
		{
			name:       "interactive create from scratch with gitignore and license",
			opts:       &CreateOptions{Interactive: true},
			tty:        true,
			wantStdout: "✓ Created repository OWNER/REPO on GitHub\n",
			promptStubs: func(p *prompter.PrompterMock) {
				p.ConfirmFunc = func(message string, defaultValue bool) (bool, error) {
					switch message {
					case "Would you like to add a README file?":
						return false, nil
					case "Would you like to add a .gitignore?":
						return true, nil
					case "Would you like to add a license?":
						return true, nil
					case `This will create "REPO" as a private repository on GitHub. Continue?`:
						return defaultValue, nil
					case "Clone the new repository locally?":
						return defaultValue, nil
					default:
						return false, fmt.Errorf("unexpected confirm prompt: %s", message)
					}
				}
				p.InputFunc = func(message, defaultValue string) (string, error) {
					switch message {
					case "Repository name":
						return "REPO", nil
					case "Description":
						return "my new repo", nil
					default:
						return "", fmt.Errorf("unexpected input prompt: %s", message)
					}
				}
				p.SelectFunc = func(message, defaultValue string, options []string) (int, error) {
					switch message {
					case "What would you like to do?":
						return prompter.IndexFor(options, "Create a new repository on GitHub from scratch")
					case "Visibility":
						return prompter.IndexFor(options, "Private")
					case "Choose a license":
						return prompter.IndexFor(options, "GNU Lesser General Public License v3.0")
					case "Choose a .gitignore template":
						return prompter.IndexFor(options, "Go")
					default:
						return 0, fmt.Errorf("unexpected select prompt: %s", message)
					}
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"someuser","organizations":{"nodes": []}}}}`))
				reg.Register(
					httpmock.REST("GET", "gitignore/templates"),
					httpmock.StringResponse(`["Actionscript","Android","AppceleratorTitanium","Autotools","Bancha","C","C++","Go"]`))
				reg.Register(
					httpmock.REST("GET", "licenses"),
					httpmock.StringResponse(`[{"key": "mit","name": "MIT License"},{"key": "lgpl-3.0","name": "GNU Lesser General Public License v3.0"}]`))
				reg.Register(
					httpmock.REST("POST", "user/repos"),
					httpmock.StringResponse(`{"name":"REPO", "owner":{"login": "OWNER"}, "html_url":"https://github.com/OWNER/REPO"}`))

			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git clone https://github.com/OWNER/REPO.git`, 0, "")
			},
		},
		{
			name:       "interactive create from scratch but with prompted owner",
			opts:       &CreateOptions{Interactive: true},
			tty:        true,
			wantStdout: "✓ Created repository org1/REPO on GitHub\n",
			promptStubs: func(p *prompter.PrompterMock) {
				p.ConfirmFunc = func(message string, defaultValue bool) (bool, error) {
					switch message {
					case "Would you like to add a README file?":
						return false, nil
					case "Would you like to add a .gitignore?":
						return false, nil
					case "Would you like to add a license?":
						return false, nil
					case `This will create "org1/REPO" as a private repository on GitHub. Continue?`:
						return true, nil
					case "Clone the new repository locally?":
						return false, nil
					default:
						return false, fmt.Errorf("unexpected confirm prompt: %s", message)
					}
				}
				p.InputFunc = func(message, defaultValue string) (string, error) {
					switch message {
					case "Repository name":
						return "REPO", nil
					case "Description":
						return "my new repo", nil
					default:
						return "", fmt.Errorf("unexpected input prompt: %s", message)
					}
				}
				p.SelectFunc = func(message, defaultValue string, options []string) (int, error) {
					switch message {
					case "Repository owner":
						return prompter.IndexFor(options, "org1")
					case "What would you like to do?":
						return prompter.IndexFor(options, "Create a new repository on GitHub from scratch")
					case "Visibility":
						return prompter.IndexFor(options, "Private")
					default:
						return 0, fmt.Errorf("unexpected select prompt: %s", message)
					}
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"someuser","organizations":{"nodes": [{"login": "org1"}, {"login": "org2"}]}}}}`))
				reg.Register(
					httpmock.REST("GET", "users/org1"),
					httpmock.StringResponse(`{"login":"org1","type":"Organization"}`))
				reg.Register(
					httpmock.GraphQL(`mutation RepositoryCreate\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"createRepository": {
								"repository": {
									"id": "REPOID",
									"name": "REPO",
									"owner": {"login":"org1"},
									"url": "https://github.com/org1/REPO"
								}
							}
						}
					}`))
			},
		},
		{
			name: "interactive create from scratch but cancel before submit",
			opts: &CreateOptions{Interactive: true},
			tty:  true,
			promptStubs: func(p *prompter.PrompterMock) {
				p.ConfirmFunc = func(message string, defaultValue bool) (bool, error) {
					switch message {
					case "Would you like to add a README file?":
						return false, nil
					case "Would you like to add a .gitignore?":
						return false, nil
					case "Would you like to add a license?":
						return false, nil
					case `This will create "REPO" as a private repository on GitHub. Continue?`:
						return false, nil
					default:
						return false, fmt.Errorf("unexpected confirm prompt: %s", message)
					}
				}
				p.InputFunc = func(message, defaultValue string) (string, error) {
					switch message {
					case "Repository name":
						return "REPO", nil
					case "Description":
						return "my new repo", nil
					default:
						return "", fmt.Errorf("unexpected input prompt: %s", message)
					}
				}
				p.SelectFunc = func(message, defaultValue string, options []string) (int, error) {
					switch message {
					case "What would you like to do?":
						return prompter.IndexFor(options, "Create a new repository on GitHub from scratch")
					case "Visibility":
						return prompter.IndexFor(options, "Private")
					default:
						return 0, fmt.Errorf("unexpected select prompt: %s", message)
					}
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"someuser","organizations":{"nodes": []}}}}`))
			},
			wantStdout: "",
			wantErr:    true,
			errMsg:     "CancelError",
		},
		{
			name: "interactive with existing repository public",
			opts: &CreateOptions{Interactive: true},
			tty:  true,
			promptStubs: func(p *prompter.PrompterMock) {
				p.ConfirmFunc = func(message string, defaultValue bool) (bool, error) {
					switch message {
					case "Add a remote?":
						return false, nil
					default:
						return false, fmt.Errorf("unexpected confirm prompt: %s", message)
					}
				}
				p.InputFunc = func(message, defaultValue string) (string, error) {
					switch message {
					case "Path to local repository":
						return defaultValue, nil
					case "Repository name":
						return "REPO", nil
					case "Description":
						return "my new repo", nil
					default:
						return "", fmt.Errorf("unexpected input prompt: %s", message)
					}
				}
				p.SelectFunc = func(message, defaultValue string, options []string) (int, error) {
					switch message {
					case "What would you like to do?":
						return prompter.IndexFor(options, "Push an existing local repository to GitHub")
					case "Visibility":
						return prompter.IndexFor(options, "Private")
					default:
						return 0, fmt.Errorf("unexpected select prompt: %s", message)
					}
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"someuser","organizations":{"nodes": []}}}}`))
				reg.Register(
					httpmock.GraphQL(`mutation RepositoryCreate\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"createRepository": {
								"repository": {
									"id": "REPOID",
									"name": "REPO",
									"owner": {"login":"OWNER"},
									"url": "https://github.com/OWNER/REPO"
								}
							}
						}
					}`))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git -C . rev-parse --git-dir`, 0, ".git")
				cs.Register(`git -C . rev-parse HEAD`, 0, "commithash")
			},
			wantStdout: "✓ Created repository OWNER/REPO on GitHub\n",
		},
		{
			name: "interactive with existing repository public add remote and push",
			opts: &CreateOptions{Interactive: true},
			tty:  true,
			promptStubs: func(p *prompter.PrompterMock) {
				p.ConfirmFunc = func(message string, defaultValue bool) (bool, error) {
					switch message {
					case "Add a remote?":
						return true, nil
					case `Would you like to push commits from the current branch to "origin"?`:
						return true, nil
					default:
						return false, fmt.Errorf("unexpected confirm prompt: %s", message)
					}
				}
				p.InputFunc = func(message, defaultValue string) (string, error) {
					switch message {
					case "Path to local repository":
						return defaultValue, nil
					case "Repository name":
						return "REPO", nil
					case "Description":
						return "my new repo", nil
					case "What should the new remote be called?":
						return defaultValue, nil
					default:
						return "", fmt.Errorf("unexpected input prompt: %s", message)
					}
				}
				p.SelectFunc = func(message, defaultValue string, options []string) (int, error) {
					switch message {
					case "What would you like to do?":
						return prompter.IndexFor(options, "Push an existing local repository to GitHub")
					case "Visibility":
						return prompter.IndexFor(options, "Private")
					default:
						return 0, fmt.Errorf("unexpected select prompt: %s", message)
					}
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"someuser","organizations":{"nodes": []}}}}`))
				reg.Register(
					httpmock.GraphQL(`mutation RepositoryCreate\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"createRepository": {
								"repository": {
									"id": "REPOID",
									"name": "REPO",
									"owner": {"login":"OWNER"},
									"url": "https://github.com/OWNER/REPO"
								}
							}
						}
					}`))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git -C . rev-parse --git-dir`, 0, ".git")
				cs.Register(`git -C . rev-parse HEAD`, 0, "commithash")
				cs.Register(`git -C . remote add origin https://github.com/OWNER/REPO`, 0, "")
				cs.Register(`git -C . push --set-upstream origin HEAD`, 0, "")
			},
			wantStdout: "✓ Created repository OWNER/REPO on GitHub\n✓ Added remote https://github.com/OWNER/REPO.git\n✓ Pushed commits to https://github.com/OWNER/REPO.git\n",
		},
		{
			name: "noninteractive create from scratch",
			opts: &CreateOptions{
				Interactive: false,
				Name:        "REPO",
				Visibility:  "PRIVATE",
			},
			tty: false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation RepositoryCreate\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"createRepository": {
								"repository": {
									"id": "REPOID",
									"name": "REPO",
									"owner": {"login":"OWNER"},
									"url": "https://github.com/OWNER/REPO"
								}
							}
						}
					}`))
			},
			wantStdout: "https://github.com/OWNER/REPO\n",
		},
		{
			name: "noninteractive create from source",
			opts: &CreateOptions{
				Interactive: false,
				Source:      ".",
				Name:        "REPO",
				Visibility:  "PRIVATE",
			},
			tty: false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation RepositoryCreate\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"createRepository": {
								"repository": {
									"id": "REPOID",
									"name": "REPO",
									"owner": {"login":"OWNER"},
									"url": "https://github.com/OWNER/REPO"
								}
							}
						}
					}`))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git -C . rev-parse --git-dir`, 0, ".git")
				cs.Register(`git -C . rev-parse HEAD`, 0, "commithash")
				cs.Register(`git -C . remote add origin https://github.com/OWNER/REPO`, 0, "")
			},
			wantStdout: "https://github.com/OWNER/REPO\n",
		},
		{
			name: "noninteractive clone from scratch",
			opts: &CreateOptions{
				Interactive: false,
				Name:        "REPO",
				Visibility:  "PRIVATE",
				Clone:       true,
			},
			tty: false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation RepositoryCreate\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"createRepository": {
								"repository": {
									"id": "REPOID",
									"name": "REPO",
									"owner": {"login":"OWNER"},
									"url": "https://github.com/OWNER/REPO"
								}
							}
						}
					}`))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git init REPO`, 0, "")
				cs.Register(`git -C REPO remote add origin https://github.com/OWNER/REPO`, 0, "")
			},
			wantStdout: "https://github.com/OWNER/REPO\n",
		},
		{
			name: "noninteractive create from template with retry",
			opts: &CreateOptions{
				Interactive: false,
				Name:        "REPO",
				Visibility:  "PRIVATE",
				Clone:       true,
				Template:    "mytemplate",
				BackOff:     &backoff.ZeroBackOff{},
			},
			tty: false,
			httpStubs: func(reg *httpmock.Registry) {
				// Test resolving repo owner from repo name only.
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"OWNER"}}}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"id": "REPOID",
								"defaultBranchRef": {
									"name": "main"
								}
							}
						}
					}`, func(s string, m map[string]interface{}) {
						assert.Equal(t, "OWNER", m["owner"])
						assert.Equal(t, "mytemplate", m["name"])
					}),
				)
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"id":"OWNERID"}}}`))
				reg.Register(
					httpmock.GraphQL(`mutation CloneTemplateRepository\b`),
					httpmock.GraphQLMutation(`
					{
						"data": {
							"cloneTemplateRepository": {
								"repository": {
									"id": "REPOID",
									"name": "REPO",
									"owner": {"login":"OWNER"},
									"url": "https://github.com/OWNER/REPO"
								}
							}
						}
					}`, func(m map[string]interface{}) {
						assert.Equal(t, "REPOID", m["repositoryId"])
					}))
			},
			execStubs: func(cs *run.CommandStubber) {
				// fatal: Remote branch main not found in upstream origin
				cs.Register(`git clone --branch main https://github.com/OWNER/REPO`, 128, "")
				cs.Register(`git clone --branch main https://github.com/OWNER/REPO`, 0, "")
			},
			wantStdout: "https://github.com/OWNER/REPO\n",
		},
	}
	for _, tt := range tests {
		prompterMock := &prompter.PrompterMock{}
		tt.opts.Prompter = prompterMock
		if tt.promptStubs != nil {
			tt.promptStubs(prompterMock)
		}

		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		tt.opts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		}

		tt.opts.GitClient = &git.Client{
			GhPath:  "some/path/gh",
			GitPath: "some/path/git",
		}

		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			cs, restoreRun := run.Stub()
			defer restoreRun(t)
			if tt.execStubs != nil {
				tt.execStubs(cs)
			}

			defer reg.Verify(t)
			err := createRun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, "", stderr.String())
		})
	}
}
