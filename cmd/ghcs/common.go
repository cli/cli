package main

// This file defines functions common to the entire ghcs command set.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/github/ghcs/api"
	"golang.org/x/term"
)

var errNoCodespaces = errors.New("You have no codespaces.")

func chooseCodespace(ctx context.Context, apiClient *api.API, user *api.User) (*api.Codespace, error) {
	codespaces, err := apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("error getting codespaces: %w", err)
	}

	if len(codespaces) == 0 {
		return nil, errNoCodespaces
	}

	sort.Slice(codespaces, func(i, j int) bool {
		return codespaces[i].CreatedAt > codespaces[j].CreatedAt
	})

	codespacesByName := make(map[string]*api.Codespace)
	codespacesNames := make([]string, 0, len(codespaces))
	for _, codespace := range codespaces {
		codespacesByName[codespace.Name] = codespace
		codespacesNames = append(codespacesNames, codespace.Name)
	}

	sshSurvey := []*survey.Question{
		{
			Name: "codespace",
			Prompt: &survey.Select{
				Message: "Choose codespace:",
				Options: codespacesNames,
				Default: codespacesNames[0],
			},
			Validate: survey.Required,
		},
	}

	var answers struct {
		Codespace string
	}
	if err := ask(sshSurvey, &answers); err != nil {
		return nil, fmt.Errorf("error getting answers: %w", err)
	}

	codespace := codespacesByName[answers.Codespace]
	return codespace, nil
}

// getOrChooseCodespace prompts the user to choose a codespace if the codespaceName is empty.
// It then fetches the codespace token and the codespace record.
func getOrChooseCodespace(ctx context.Context, apiClient *api.API, user *api.User, codespaceName string) (codespace *api.Codespace, token string, err error) {
	if codespaceName == "" {
		codespace, err = chooseCodespace(ctx, apiClient, user)
		if err != nil {
			if err == errNoCodespaces {
				return nil, "", err
			}
			return nil, "", fmt.Errorf("choosing codespace: %w", err)
		}
		codespaceName = codespace.Name

		token, err = apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
		if err != nil {
			return nil, "", fmt.Errorf("getting codespace token: %w", err)
		}
	} else {
		token, err = apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
		if err != nil {
			return nil, "", fmt.Errorf("getting codespace token for given codespace: %w", err)
		}

		codespace, err = apiClient.GetCodespace(ctx, token, user.Login, codespaceName)
		if err != nil {
			return nil, "", fmt.Errorf("getting full codespace details: %w", err)
		}
	}

	return codespace, token, nil
}

// hasTTY indicates whether the process connected to a terminal.
// It is not portable to assume stdin/stdout are fds 0 and 1.
var hasTTY = term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))

// ask asks survey questions on the terminal, using standard options.
// It fails unless hasTTY, but ideally callers should avoid calling it in that case.
func ask(qs []*survey.Question, response interface{}) error {
	if !hasTTY {
		return fmt.Errorf("no terminal")
	}
	err := survey.Ask(qs, response, survey.WithShowCursor(true))
	// The survey package temporarily clears the terminal's ISIG mode bit
	// (see tcsetattr(3)) so the QUIT button (Ctrl-C) is reported as
	// ASCII \x03 (ETX) instead of delivering SIGINT to the application.
	// So we have to serve ourselves the SIGINT.
	//
	// https://github.com/AlecAivazis/survey/#why-isnt-ctrl-c-working
	if err == terminal.InterruptErr {
		self, _ := os.FindProcess(os.Getpid())
		_ = self.Signal(os.Interrupt) // assumes POSIX

		// Suspend the goroutine, to avoid a race between
		// return from main and async delivery of INT signal.
		select {}
	}
	return err
}
