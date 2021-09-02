package liveshare

import (
	"context"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

type Terminal struct {
	session *Session
}

func NewTerminal(session *Session) *Terminal {
	return &Terminal{session: session}
}

type TerminalCommand struct {
	terminal *Terminal
	cwd      string
	cmd      string
}

func (t *Terminal) NewCommand(cwd, cmd string) TerminalCommand {
	return TerminalCommand{t, cwd, cmd}
}

type runArgs struct {
	Name              string   `json:"name"`
	Rows              int      `json:"rows"`
	Cols              int      `json:"cols"`
	App               string   `json:"app"`
	Cwd               string   `json:"cwd"`
	CommandLine       []string `json:"commandLine"`
	ReadOnlyForGuests bool     `json:"readOnlyForGuests"`
}

type startTerminalResult struct {
	ID              int    `json:"id"`
	StreamName      string `json:"streamName"`
	StreamCondition string `json:"streamCondition"`
	LocalPipeName   string `json:"localPipeName"`
	AppProcessID    int    `json:"appProcessId"`
}

type getStreamArgs struct {
	StreamName string `json:"streamName"`
	Condition  string `json:"condition"`
}

type stopTerminalArgs struct {
	ID int `json:"id"`
}

func (t TerminalCommand) Run(ctx context.Context) (io.ReadCloser, error) {
	args := runArgs{
		Name:              "RunCommand",
		Rows:              10,
		Cols:              80,
		App:               "/bin/bash",
		Cwd:               t.cwd,
		CommandLine:       []string{"-c", t.cmd},
		ReadOnlyForGuests: false,
	}

	terminalStarted := t.terminal.session.rpc.handler.registerEventHandler("terminal.terminalStarted")
	var result startTerminalResult
	if err := t.terminal.session.rpc.do(ctx, "terminal.startTerminal", &args, &result); err != nil {
		return nil, fmt.Errorf("error making terminal.startTerminal call: %v", err)
	}
	<-terminalStarted

	channel, err := t.terminal.session.openStreamingChannel(ctx, result.StreamName, result.StreamCondition)
	if err != nil {
		return nil, fmt.Errorf("error opening streaming channel: %v", err)
	}

	return t.newTerminalReadCloser(result.ID, channel), nil
}

type terminalReadCloser struct {
	terminalCommand TerminalCommand
	terminalID      int
	channel         ssh.Channel
}

func (t TerminalCommand) newTerminalReadCloser(terminalID int, channel ssh.Channel) io.ReadCloser {
	return terminalReadCloser{t, terminalID, channel}
}

func (t terminalReadCloser) Read(b []byte) (int, error) {
	return t.channel.Read(b)
}

func (t terminalReadCloser) Close() error {
	terminalStopped := t.terminalCommand.terminal.session.rpc.handler.registerEventHandler("terminal.terminalStopped")
	if err := t.terminalCommand.terminal.session.rpc.do(context.Background(), "terminal.stopTerminal", []int{t.terminalID}, nil); err != nil {
		return fmt.Errorf("error making terminal.stopTerminal call: %v", err)
	}

	if err := t.channel.Close(); err != nil && err != io.EOF {
		return fmt.Errorf("error closing channel: %v", err)
	}

	<-terminalStopped

	return nil
}
