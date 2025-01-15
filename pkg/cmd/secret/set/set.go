package set

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/secret/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/hashicorp/go-multierror"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/nacl/box"
)

type SetOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter

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
	Application     string
}

type iprompter interface {
	Password(string) (string, error)
}

func NewCmdSet(f *cmdutil.Factory, runF func(*SetOptions) error) *cobra.Command {
	opts := &SetOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "set <secret-name>",
		Short: "Create or update secrets",
		Long: heredoc.Doc(`
			Set a value for a secret on one of the following levels:
			- repository (default): available to GitHub Actions runs or Dependabot in a repository
			- environment: available to GitHub Actions runs for a deployment environment in a repository
			- organization: available to GitHub Actions runs, Dependabot, or Codespaces within an organization
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

			# Set repository-level secret for Dependabot
			$ gh secret set MYSECRET --app dependabot

			# Set multiple secrets imported from the ".env" file
			$ gh secret set -f .env

			# Set multiple secrets from stdin
			$ gh secret set -f - < myfile.txt
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
	cmdutil.StringEnumFlag(cmd, &opts.Visibility, "visibility", "v", shared.Private, []string{shared.All, shared.Private, shared.Selected}, "Set visibility for an organization secret")
	cmd.Flags().StringSliceVarP(&opts.RepositoryNames, "repos", "r", []string{}, "List of `repositories` that can access an organization or user secret")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The value for the secret (reads from standard input if not specified)")
	cmd.Flags().BoolVar(&opts.DoNotStore, "no-store", false, "Print the encrypted, base64-encoded value instead of storing it on GitHub")
	cmd.Flags().StringVarP(&opts.EnvFile, "env-file", "f", "", "Load secret names and values from a dotenv-formatted `file`")
	cmdutil.StringEnumFlag(cmd, &opts.Application, "app", "a", "", []string{shared.Actions, shared.Codespaces, shared.Dependabot}, "Set the application for a secret")

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
			return err
		}
		host = baseRepo.RepoHost()
	} else {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}
		host, _ = cfg.Authentication().DefaultHost()
	}

	secretEntity, err := shared.GetSecretEntity(orgName, envName, opts.UserSecrets)
	if err != nil {
		return err
	}

	secretApp, err := shared.GetSecretApp(opts.Application, secretEntity)
	if err != nil {
		return err
	}

	if !shared.IsSupportedSecretEntity(secretApp, secretEntity) {
		return fmt.Errorf("%s secrets are not supported for %s", secretEntity, secretApp)
	}

	var pk *PubKey
	switch secretEntity {
	case shared.Organization:
		pk, err = getOrgPublicKey(client, host, orgName, secretApp)
	case shared.Environment:
		pk, err = getEnvPubKey(client, baseRepo, envName)
	case shared.User:
		pk, err = getUserPublicKey(client, host)
	default:
		pk, err = getRepoPubKey(client, baseRepo, secretApp)
	}
	if err != nil {
		return fmt.Errorf("failed to fetch public key: %w", err)
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
			setc <- setSecret(opts, pk, host, client, baseRepo, key, value, repositoryIDs, secretApp, secretEntity)
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
		fmt.Fprintf(opts.IO.Out, "%s Set %s secret %s for %s\n", cs.SuccessIcon(), secretApp.Title(), result.key, target)
	}
	return err
}

type setResult struct {
	key       string
	encrypted string
	err       error
}

func setSecret(opts *SetOptions, pk *PubKey, host string, client *api.Client, baseRepo ghrepo.Interface, secretKey string, secret []byte, repositoryIDs []int64, app shared.App, entity shared.SecretEntity) (res setResult) {
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

	switch entity {
	case shared.Organization:
		err = putOrgSecret(client, host, pk, orgName, opts.Visibility, secretKey, encoded, repositoryIDs, app)
	case shared.Environment:
		err = putEnvSecret(client, pk, baseRepo, envName, secretKey, encoded)
	case shared.User:
		err = putUserSecret(client, host, pk, secretKey, encoded, repositoryIDs)
	default:
		err = putRepoSecret(client, pk, baseRepo, secretKey, encoded, app)
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
		bodyInput, err := opts.Prompter.Password("Paste your secret:")
		if err != nil {
			return nil, err
		}
		fmt.Fprintln(opts.IO.Out)
		return []byte(bodyInput), nil
	}

	body, err := io.ReadAll(opts.IO.In)
	if err != nil {
		return nil, fmt.Errorf("failed to read from standard input: %w", err)
	}

	return bytes.TrimRight(body, "\r\n"), nil
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
	repositoryIDs, err := api.GetRepoIDs(client, host, repos)
	if err != nil {
		return nil, fmt.Errorf("failed to look up IDs for repositories %v: %w", repositoryNames, err)
	}
	return repositoryIDs, nil
}
