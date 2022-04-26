module github.com/cli/cli/v2

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.3.4
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/briandowns/spinner v1.18.1
	github.com/charmbracelet/glamour v0.4.0
	github.com/charmbracelet/lipgloss v0.5.0
	github.com/cli/browser v1.1.0
	github.com/cli/oauth v0.9.0
	github.com/cli/safeexec v1.0.0
	github.com/cli/shurcooL-graphql v0.0.1
	github.com/cpuguy83/go-md2man/v2 v2.0.2
	github.com/creack/pty v1.1.18
	github.com/gabriel-vasile/mimetype v1.4.0
	github.com/google/go-cmp v0.5.7
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-version v1.3.0
	github.com/henvic/httpretty v0.0.6
	github.com/itchyny/gojq v0.12.7
	github.com/joho/godotenv v1.4.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/mattn/go-colorable v0.1.12
	github.com/mattn/go-isatty v0.0.14
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/muesli/reflow v0.3.0
	github.com/muesli/termenv v0.11.1-0.20220204035834-5ac8409525e0
	github.com/muhammadmuzzammil1998/jsonc v0.0.0-20201229145248-615b0916ca38
	github.com/opentracing/opentracing-go v1.1.0
	github.com/shurcooL/githubv4 v0.0.0-20200928013246-d292edc3691b
	github.com/shurcooL/graphql v0.0.0-20200928012149-18c5c3165e3a // indirect
	github.com/sourcegraph/jsonrpc2 v0.1.0
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.1
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5
	golang.org/x/oauth2 v0.0.0-20220309155454-6242fa91716a // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20220227234510-4e6760a101f9
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

replace golang.org/x/crypto => github.com/cli/crypto v0.0.0-20210929142629-6be313f59b03
