// Package engine owns CLI exercise policy, controllers, and capture in Go.
package engine

import "github.com/cli/cli/v2/cli-exercise/internal/terminal"

// Options supplies explicit case authority and a mechanical terminal adapter.
type Options struct {
	SkillRoot         string
	ContractPath      string
	PassedEnvironment map[string]string
	OperatorPaths     []string
	OperatorHomes     []string
	SystemEnvironment map[string]string
	Terminal          terminal.Session
}
