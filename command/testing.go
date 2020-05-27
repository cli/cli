package command

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/core"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/google/shlex"
	"github.com/spf13/pflag"
)

const defaultTestConfig = `hosts:
  github.com:
    user: OWNER
    oauth_token: 1234567890
`

type askStubber struct {
	Asks  [][]*survey.Question
	Count int
	Stubs [][]*QuestionStub
}

func initAskStubber() (*askStubber, func()) {
	origSurveyAsk := SurveyAsk
	as := askStubber{}
	SurveyAsk = func(qs []*survey.Question, response interface{}, opts ...survey.AskOpt) error {
		as.Asks = append(as.Asks, qs)
		count := as.Count
		as.Count += 1
		if count >= len(as.Stubs) {
			panic(fmt.Sprintf("more asks than stubs. most recent call: %v", qs))
		}

		// actually set response
		stubbedQuestions := as.Stubs[count]
		for i, sq := range stubbedQuestions {
			q := qs[i]
			if q.Name != sq.Name {
				panic(fmt.Sprintf("stubbed question mismatch: %s != %s", q.Name, sq.Name))
			}
			if sq.Default {
				defaultValue := reflect.ValueOf(q.Prompt).Elem().FieldByName("Default")
				_ = core.WriteAnswer(response, q.Name, defaultValue)
			} else {
				_ = core.WriteAnswer(response, q.Name, sq.Value)
			}
		}

		return nil
	}
	teardown := func() {
		SurveyAsk = origSurveyAsk
	}
	return &as, teardown
}

type QuestionStub struct {
	Name    string
	Value   interface{}
	Default bool
}

func (as *askStubber) Stub(stubbedQuestions []*QuestionStub) {
	// A call to .Ask takes a list of questions; a stub is then a list of questions in the same order.
	as.Stubs = append(as.Stubs, stubbedQuestions)
}

func initBlankContext(cfg, repo, branch string) {
	initContext = func() context.Context {
		ctx := context.NewBlank()
		ctx.SetBaseRepo(repo)
		ctx.SetBranch(branch)
		ctx.SetRemotes(map[string]string{
			"origin": "OWNER/REPO",
		})

		if cfg == "" {
			cfg = defaultTestConfig
		}

		// NOTE we are not restoring the original readConfig; we never want to touch the config file on
		// disk during tests.
		config.StubConfig(cfg)

		return ctx
	}
}

func initFakeHTTP() *httpmock.Registry {
	http := &httpmock.Registry{}
	ensureScopes = func(ctx context.Context, client *api.Client, wantedScopes ...string) (*api.Client, error) {
		return client, nil
	}
	apiClientForContext = func(context.Context) (*api.Client, error) {
		return api.NewClient(api.ReplaceTripper(http)), nil
	}
	return http
}

type cmdOut struct {
	outBuf, errBuf *bytes.Buffer
}

func (c cmdOut) String() string {
	return c.outBuf.String()
}

func (c cmdOut) Stderr() string {
	return c.errBuf.String()
}

func RunCommand(args string) (*cmdOut, error) {
	rootCmd := RootCmd
	rootArgv, err := shlex.Split(args)
	if err != nil {
		return nil, err
	}

	cmd, _, err := rootCmd.Traverse(rootArgv)
	if err != nil {
		return nil, err
	}

	rootCmd.SetArgs(rootArgv)

	outBuf := bytes.Buffer{}
	cmd.SetOut(&outBuf)
	errBuf := bytes.Buffer{}
	cmd.SetErr(&errBuf)

	// Reset flag values so they don't leak between tests
	// FIXME: change how we initialize Cobra commands to render this hack unnecessary
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		switch v := f.Value.(type) {
		case pflag.SliceValue:
			_ = v.Replace([]string{})
		default:
			switch v.Type() {
			case "bool", "string", "int":
				_ = v.Set(f.DefValue)
			}
		}
	})

	_, err = rootCmd.ExecuteC()
	cmd.SetOut(nil)
	cmd.SetErr(nil)

	return &cmdOut{&outBuf, &errBuf}, err
}

type errorStub struct {
	message string
}

func (s errorStub) Output() ([]byte, error) {
	return nil, errors.New(s.message)
}

func (s errorStub) Run() error {
	return errors.New(s.message)
}
