package cobra

import "fmt"

type PositionalArgs func(cmd *Command, args []string) error

func MinimumNArgs(n int) PositionalArgs {
	return func(cmd *Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf("requires at least %d arguments, got %d", n, len(args))
		}
		return nil
	}
}

func MaximumNArgs(n int) PositionalArgs {
	return func(cmd *Command, args []string) error {
		if len(args) > n {
			return fmt.Errorf("accepts at most %d arguments, got %d", n, len(args))
		}
		return nil
	}
}

func ExactArgs(n int) PositionalArgs {
	return func(cmd *Command, args []string) error {
		if len(args) > n {
			return fmt.Errorf("accepts exactly %d arguments, got %d", n, len(args))
		}
		return nil
	}
}

var NoArgs PositionalArgs = func(cmd *Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("does not accept any arguments, got %v", args)
	}
	return nil
}
