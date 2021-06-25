package liveshare

import (
	"context"
	"fmt"
	"net/rpc"
)

type LiveShare struct {
	Configuration *Configuration

	workspaceClient *Client
	terminal        *Terminal
}

func New(opts ...Option) (*LiveShare, error) {
	configuration := NewConfiguration()

	for _, o := range opts {
		if err := o(configuration); err != nil {
			return nil, fmt.Errorf("error configuring liveshare: %v", err)
		}
	}

	if err := configuration.Validate(); err != nil {
		return nil, fmt.Errorf("error validating configuration: %v", err)
	}

	return &LiveShare{Configuration: configuration}, nil
}

func (l *LiveShare) Connect(ctx context.Context) error {
	l.workspaceClient = NewClient(l.Configuration)
	if err := l.workspaceClient.Join(ctx); err != nil {
		return fmt.Errorf("error joining with workspace client: %v", err)
	}

	return nil
}

type Terminal struct {
	WorkspaceClient *Client
	RPCClient       *rpc.Client
}

func (l *LiveShare) NewTerminal() *Terminal {
	return &Terminal{
		WorkspaceClient: l.workspaceClient,
		RPCClient:       rpc.NewClient(l.workspaceClient.SSHSession),
	}
}

type TerminalCommand struct {
	Terminal *Terminal
	Cwd      string
	Cmd      string
}

func (t *Terminal) NewCommand(cwd, cmd string) TerminalCommand {
	return TerminalCommand{t, cwd, cmd}
}

type RunArgs struct {
	Name              string
	Rows, Cols        int
	App               string
	Cwd               string
	CommandLine       []string
	ReadOnlyForGuests bool
}

func (t TerminalCommand) Run(ctx context.Context) ([]byte, error) {
	args := RunArgs{
		Name:              "RunCommand",
		Rows:              10,
		Cols:              80,
		App:               "/bin/bash",
		Cwd:               t.Cwd,
		CommandLine:       []string{"-c", t.Cmd},
		ReadOnlyForGuests: false,
	}

	var output []byte
	runCall := t.Terminal.RPCClient.Go("terminal.startAsync", &args, &output, nil)

	runReply := <-runCall.Done
	if runReply.Error != nil {
		return nil, fmt.Errorf("error startAsync operation: %v", runReply.Error)
	}
	fmt.Printf("%+v\n\n", runReply)
	return output, nil
}
