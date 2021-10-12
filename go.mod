module github.com/cli/cli/v2

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.3.2
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/briandowns/spinner v1.16.0
	github.com/charmbracelet/glamour v0.3.0
	github.com/cli/browser v1.1.0
	github.com/cli/oauth v0.8.0
	github.com/cli/safeexec v1.0.0
	github.com/cpuguy83/go-md2man/v2 v2.0.1
	github.com/creack/pty v1.1.16
	github.com/fatih/camelcase v1.0.0
	github.com/gabriel-vasile/mimetype v1.1.2
	github.com/google/go-cmp v0.5.6
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/go-version v1.3.0
	github.com/henvic/httpretty v0.0.6
	github.com/itchyny/gojq v0.12.5
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/mattn/go-colorable v0.1.11
	github.com/mattn/go-isatty v0.0.14
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/muesli/reflow v0.2.1-0.20210502190812-c80126ec2ad5
	github.com/muesli/termenv v0.9.0
	github.com/muhammadmuzzammil1998/jsonc v0.0.0-20201229145248-615b0916ca38
	github.com/olekukonko/tablewriter v0.0.5
	github.com/opentracing/opentracing-go v1.2.0
	github.com/shurcooL/githubv4 v0.0.0-20200928013246-d292edc3691b
	github.com/shurcooL/graphql v0.0.0-20181231061246-d48a9a75455f
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/sourcegraph/jsonrpc2 v0.1.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/objx v0.1.1 // indirect
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210927094055-39ccf1dd6fa6
	golang.org/x/term v0.0.0-20210503060354-a79de5458b56
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

replace github.com/shurcooL/graphql => github.com/cli/shurcooL-graphql v0.0.0-20200707151639-0f7232a2bf7e

replace golang.org/x/crypto => github.com/cli/crypto v0.0.0-20210929142629-6be313f59b03
