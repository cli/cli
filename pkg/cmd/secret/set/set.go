package set

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/secret/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/hashicorp/go-multierror"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/nacl/box"
)

type SetOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)

	RandomOverride func() io.Reader

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

			if err := cmdutil.MutuallyExclusive("specify only one of `--env-file` or `--no-store`", opts.EnvFile != "", opts.DoNotStore); err != nil {
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

	var host string
	var baseRepo ghrepo.Interface
	if orgName == "" && !opts.UserSecrets {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return fmt.Errorf("could not determine base repo: %w", err)
		}
		host = baseRepo.RepoHost()
	} else {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}

		host, err = cfg.DefaultHost()
		if err != nil {
			return err
		}
	}

	type repoNamesResult struct {
		ids []int64
		err error
	}
	repoNamesC := make(chan repoNamesResult, 1)
	go func() {
		if len(opts.RepositoryNames) == 0 {
			repoNamesC <- repoNamesResult{}
			return
		}
		repositoryIDs, err := mapRepoNamesToIDs(client, host, opts.OrgName, opts.RepositoryNames)
		repoNamesC <- repoNamesResult{
			ids: repositoryIDs,
			err: err,
		}
	}()

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

	var repositoryIDs []int64
	if result := <-repoNamesC; result.err == nil {
		repositoryIDs = result.ids
	} else {
		return result.err
	}

	setc := make(chan setResult)
	for secretKey, secret := range secrets {
		key := secretKey
		value := secret
		go func() {
			setc <- setSecret(opts, pk, host, client, baseRepo, key, value, repositoryIDs)
		}()
	}

	err = nil
	cs := opts.IO.ColorScheme()
	for i := 0; i < len(secrets); i++ {
		result := <-setc
		if result.err != nil {
			err = multierror.Append(err, result.err)
			continue
		}
		if result.encrypted != "" {
			fmt.Fprintln(opts.IO.Out, result.encrypted)
			continue
		}
		if !opts.IO.IsStdoutTTY() {
			continue
		}
		target := orgName
		if opts.UserSecrets {
			target = "your user"
		} else if orgName == "" {
			target = ghrepo.FullName(baseRepo)
		}
		fmt.Fprintf(opts.IO.Out, "%s Set secret %s for %s\n", cs.SuccessIcon(), result.key, target)
	}
	return err
}

type setResult struct {
	key       string
	encrypted string
	err       error
}

func setSecret(opts *SetOptions, pk *PubKey, host string, client *api.Client, baseRepo ghrepo.Interface, secretKey string, secret []byte, repositoryIDs []int64) (res setResult) {
	orgName := opts.OrgName
	envName := opts.EnvName
	res.key = secretKey

	decodedPubKey, err := base64.StdEncoding.DecodeString(pk.Key)
	if err != nil {
		res.err = fmt.Errorf("failed to decode public key: %w", err)
		return
	}
	var peersPubKey [32]byte
	copy(peersPubKey[:], decodedPubKey[0:32])

	var rand io.Reader
	if opts.RandomOverride != nil {
		rand = opts.RandomOverride()
	}
	eBody, err := box.SealAnonymous(nil, secret[:], &peersPubKey, rand)
	if err != nil {
		res.err = fmt.Errorf("failed to encrypt body: %w", err)
		return
	}

	encoded := base64.StdEncoding.EncodeToString(eBody)
	if opts.DoNotStore {
		res.encrypted = encoded
		return
	}

	if orgName != "" {
		err = putOrgSecret(client, host, pk, opts.OrgName, opts.Visibility, secretKey, encoded, repositoryIDs)
	} else if envName != "" {
		err = putEnvSecret(client, pk, baseRepo, envName, secretKey, encoded)
	} else if opts.UserSecrets {
		err = putUserSecret(client, host, pk, secretKey, encoded, repositoryIDs)
	} else {
		err = putRepoSecret(client, pk, baseRepo, secretKey, encoded)
	}
	if err != nil {
		res.err = fmt.Errorf("failed to set secret %q: %w", secretKey, err)
		return
	}
	return
}

func getSecretsFromOptions(opts *SetOptions) (map[string][]byte, error) {
	secrets := make(map[string][]byte)

	if opts.EnvFile != "" {
		var r io.Reader
		if opts.EnvFile == "-" {
			defer opts.IO.In.Close()
			r = opts.IO.In
		} else {
			f, err := os.Open(opts.EnvFile)
			if err != nil {
				return nil, fmt.Errorf("failed to open env file: %w", err)
			}
			defer f.Close()
			r = f
		}
		envs, err := godotenv.Parse(r)
		if err != nil {
			return nil, fmt.Errorf("error parsing env file: %w", err)
		}
		if len(envs) == 0 {
			return nil, fmt.Errorf("no secrets found in file")
		}
		for key, value := range envs {
			secrets[key] = []byte(value)
		}
		return secrets, nil
	}

	body, err := getBody(opts)
	if err != nil {
		return nil, fmt.Errorf("did not understand secret body: %w", err)
	}
	secrets[opts.SecretName] = body
	return secrets, nil
}

func getBody(opts *SetOptions) ([]byte, error) {
	if opts.Body != "" {
		return []byte(opts.Body), nil
	}

	if opts.IO.CanPrompt() {
		var bodyInput string
		err := prompt.SurveyAskOne(&survey.Password{
			Message: "Paste your secret",
		}, &bodyInput)
		if err != nil {
			return nil, err
		}
		fmt.Fprintln(opts.IO.Out)
		return []byte(bodyInput), nil
	}

	body, err := ioutil.ReadAll(opts.IO.In)
	if err != nil {
		return nil, fmt.Errorf("failed to read from standard input: %w", err)
	}

	return body, nil
}

func mapRepoNamesToIDs(client *api.Client, host, defaultOwner string, repositoryNames []string) ([]int64, error) {
	repos := make([]ghrepo.Interface, 0, len(repositoryNames))
	for _, repositoryName := range repositoryNames {
		var repo ghrepo.Interface
		if strings.Contains(repositoryName, "/") || defaultOwner == "" {
			var err error
			repo, err = ghrepo.FromFullNameWithHost(repositoryName, host)
			if err != nil {
				return nil, fmt.Errorf("invalid repository name: %w", err)
			}
		} else {
			repo = ghrepo.NewWithHost(defaultOwner, repositoryName, host)
		}
		repos = append(repos, repo)
	}
	repositoryIDs, err := mapRepoToID(client, host, repos)
	if err != nil {
		return nil, fmt.Errorf("failed to look up IDs for repositories %v: %w", repositoryNames, err)
	}
	return repositoryIDs, nil
}
