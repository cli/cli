package set

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/secret/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/nacl/box"
)

type SetOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)

	RandomOverride io.Reader

	SecretName      string
	OrgName         string
	EnvName         string
	UserSecrets     bool
	Body            string
	DoNotStore      bool
	Visibility      string
	RepositoryNames []string
	EnvFile         string
}

func NewCmdSet(f *cmdutil.Factory, runF func(*SetOptions) error) *cobra.Command {
	opts := &SetOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "set <secret-name>",
		Short: "Create or update secrets",
		Long: heredoc.Doc(`
			Set a value for a secret on one of the following levels:
			- repository (default): available to Actions runs in a repository
			- environment: available to Actions runs for a deployment environment in a repository
			- organization: available to Actions runs within an organization
			- user: available to Codespaces for your user

			Organization and user secrets can optionally be restricted to only be available to
			specific repositories.

			Secret values are locally encrypted before being sent to GitHub.
		`),
		Example: heredoc.Doc(`
			# Paste secret value for the current repository in an interactive prompt
			$ gh secret set MYSECRET

			# Read secret value from an environment variable
			$ gh secret set MYSECRET --body "$ENV_VALUE"

			# Read secret value from a file
			$ gh secret set MYSECRET < myfile.txt

			# Set secret for a deployment environment in the current repository
			$ gh secret set MYSECRET --env myenvironment

			# Set organization-level secret visible to both public and private repositories
			$ gh secret set MYSECRET --org myOrg --visibility all

			# Set organization-level secret visible to specific repositories
			$ gh secret set MYSECRET --org myOrg --repos repo1,repo2,repo3

			# Set user-level secret for Codespaces
			$ gh secret set MYSECRET --user

			# Set multiple secrets imported from the ".env" file
			$ gh secret set -f .env
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--org`, `--env`, or `--user`", opts.OrgName != "", opts.EnvName != "", opts.UserSecrets); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive("specify only one of `--body` or `--env-file`", opts.Body != "", opts.EnvFile != ""); err != nil {
				return err
			}

			if len(args) == 0 {
				if !opts.DoNotStore && opts.EnvFile == "" {
					return cmdutil.FlagErrorf("must pass name argument")
				}
			} else {
				opts.SecretName = args[0]
			}

			if cmd.Flags().Changed("visibility") {
				if opts.OrgName == "" {
					return cmdutil.FlagErrorf("`--visibility` is only supported with `--org`")
				}

				if opts.Visibility != shared.All && opts.Visibility != shared.Private && opts.Visibility != shared.Selected {
					return cmdutil.FlagErrorf("`--visibility` must be one of \"all\", \"private\", or \"selected\"")
				}

				if opts.Visibility != shared.Selected && len(opts.RepositoryNames) > 0 {
					return cmdutil.FlagErrorf("`--repos` is only supported with `--visibility=selected`")
				}

				if opts.Visibility == shared.Selected && len(opts.RepositoryNames) == 0 {
					return cmdutil.FlagErrorf("`--repos` list required with `--visibility=selected`")
				}
			} else {
				if len(opts.RepositoryNames) > 0 {
					opts.Visibility = shared.Selected
				}
			}

			if runF != nil {
				return runF(opts)
			}

			return setRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "Set `organization` secret")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "Set deployment `environment` secret")
	cmd.Flags().BoolVarP(&opts.UserSecrets, "user", "u", false, "Set a secret for your user")
	cmd.Flags().StringVarP(&opts.Visibility, "visibility", "v", "private", "Set visibility for an organization secret: `{all|private|selected}`")
	cmd.Flags().StringSliceVarP(&opts.RepositoryNames, "repos", "r", []string{}, "List of `repositories` that can access an organization or user secret")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The value for the secret (reads from standard input if not specified)")
	cmd.Flags().BoolVar(&opts.DoNotStore, "no-store", false, "Print the encrypted, base64-encoded value instead of storing it on Github")
	cmd.Flags().StringVarP(&opts.EnvFile, "env-file", "f", "", "Load secret names and values from a dotenv-formatted `file`")

	return cmd
}

func setRun(opts *SetOptions) error {
	secrets, err := getSecretsFromOptions(opts)
	if err != nil {
		return err
	}

	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	orgName := opts.OrgName
	envName := opts.EnvName

	var baseRepo ghrepo.Interface
	if orgName == "" && !opts.UserSecrets {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return fmt.Errorf("could not determine base repo: %w", err)
		}
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, err := cfg.DefaultHost()
	if err != nil {
		return err
	}

	var pk *PubKey
	if orgName != "" {
		pk, err = getOrgPublicKey(client, host, orgName)
	} else if envName != "" {
		pk, err = getEnvPubKey(client, baseRepo, envName)
	} else if opts.UserSecrets {
		pk, err = getUserPublicKey(client, host)
	} else {
		pk, err = getRepoPubKey(client, baseRepo)
	}
	if err != nil {
		return fmt.Errorf("failed to fetch public key: %w", err)
	}

	for secretKey, secret := range secrets {
		// for env files the randomoverride used in tests needs to be reset on every iteration
		if len(opts.EnvFile) > 0 {
			if rd, ok := opts.RandomOverride.(io.Seeker); ok {
				if _, err := rd.Seek(0, 0); err != nil {
					return fmt.Errorf("internal error: %w", err)
				}
			}
		}
		eBody, err := box.SealAnonymous(nil, secret[:], &pk.Raw, opts.RandomOverride)
		if err != nil {
			return fmt.Errorf("failed to encrypt body: %w", err)
		}

		encoded := base64.StdEncoding.EncodeToString(eBody)
		if opts.DoNotStore {
			_, err := fmt.Fprintln(opts.IO.Out, encoded)
			return err
		}

		if orgName != "" {
			err = putOrgSecret(client, host, pk, *opts, secretKey, encoded)
		} else if envName != "" {
			err = putEnvSecret(client, pk, baseRepo, envName, secretKey, encoded)
		} else if opts.UserSecrets {
			err = putUserSecret(client, host, pk, *opts, encoded)
		} else {
			err = putRepoSecret(client, pk, baseRepo, secretKey, encoded)
		}
		if err != nil {
			return fmt.Errorf("failed to set secret: %w", err)
		}

		if opts.IO.IsStdoutTTY() {
			target := orgName
			if opts.UserSecrets {
				target = "your user"
			} else if orgName == "" {
				target = ghrepo.FullName(baseRepo)
			}
			cs := opts.IO.ColorScheme()
			fmt.Fprintf(opts.IO.Out, "%s Set secret %s for %s\n", cs.SuccessIconWithColor(cs.Green), secretKey, target)
		}
	}
	return nil
}

func getSecretsFromOptions(opts *SetOptions) (map[string][]byte, error) {
	secrets := make(map[string][]byte)

	if len(opts.EnvFile) > 0 {
		envs, err := godotenv.Read(opts.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("could no open env file: %w", err)
		}
		for key, value := range envs {
			secrets[key] = []byte(value)
		}

	} else {
		body, err := getBody(opts)
		if err != nil {
			return nil, fmt.Errorf("did not understand secret body: %w", err)
		}
		secrets[opts.SecretName] = body
	}

	if len(secrets) == 0 {
		return nil, fmt.Errorf("no secrets defined")
	}
	return secrets, nil
}

func getBody(opts *SetOptions) ([]byte, error) {
	if opts.Body == "" {
		if opts.IO.CanPrompt() {
			err := prompt.SurveyAskOne(&survey.Password{
				Message: "Paste your secret",
			}, &opts.Body)
			if err != nil {
				return nil, err
			}
			fmt.Fprintln(opts.IO.Out)
		} else {
			body, err := ioutil.ReadAll(opts.IO.In)
			if err != nil {
				return nil, fmt.Errorf("failed to read from STDIN: %w", err)
			}

			return body, nil
		}
	}

	return []byte(opts.Body), nil
}
