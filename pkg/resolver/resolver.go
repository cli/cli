package resolver

import (
	"errors"
	"os"

	"github.com/cli/cli/internal/config"
	"github.com/spf13/cobra"
)

type Strat func() (interface{}, error)

type ResolverFunction func() (interface{}, error)

type Resolver struct {
	strats map[string][]Strat
}

func New() *Resolver {
	return &Resolver{
		strats: make(map[string][]Strat),
	}
}

func (r *Resolver) AddStrat(name string, strat Strat) {
	r.strats[name] = append(r.strats[name], strat)
}

func (r *Resolver) GetStrats(name string) []Strat {
	return r.strats[name]
}

func (r *Resolver) ResolverFunc(name string) ResolverFunction {
	return ResolverFunc(r.GetStrats(name)...)
}

func (r *Resolver) Resolve(name string) (interface{}, error) {
	return ResolverFunc(r.GetStrats(name)...)()
}

func ResolverFunc(strategies ...Strat) ResolverFunction {
	return func() (interface{}, error) {
		for _, strat := range strategies {
			out, err := strat()
			if err != nil {
				return nil, err
			}

			if out != nil {
				return out, nil
			}
		}

		return "", errors.New("No strategies resulted in output")
	}
}

func EnvStrat(name string) Strat {
	return func() (interface{}, error) {
		val, ok := os.LookupEnv(name)
		if !ok {
			return nil, nil
		} else {
			return val, nil
		}
	}
}

func ConfigStrat(config func() (config.Config, error), hostname, name string) Strat {
	return func() (interface{}, error) {
		cfg, err := config()
		if err != nil {
			return nil, err
		}

		return cfg.Get(hostname, name)
	}
}

func ArgsStrat(args []string, index int) Strat {
	return func() (interface{}, error) {
		if len(args) > index {
			return args[index], nil
		}
		return nil, nil
	}
}

func StringFlagStrat(cmd *cobra.Command, name string) Strat {
	return func() (interface{}, error) {
		if !cmd.Flags().Changed(name) {
			return nil, nil
		}
		return cmd.Flags().GetString(name)
	}
}

func BoolFlagStrat(cmd *cobra.Command, name string) Strat {
	return func() (interface{}, error) {
		return cmd.Flags().GetBool(name)
	}
}

func StringSliceFlagsStrat(cmd *cobra.Command, name string) Strat {
	return func() (interface{}, error) {
		if !cmd.Flags().Changed(name) {
			return nil, nil
		}
		return cmd.Flags().GetStringSlice(name)
	}
}

func MutuallyExclusiveBoolFlagsStrat(cmd *cobra.Command, names ...string) Strat {
	return func() (interface{}, error) {
		flags := cmd.Flags()
		enabledFlagCount := 0
		enabledFlag := ""
		for _, name := range names {
			val, err := flags.GetBool(name)
			if err != nil {
				return nil, err
			}

			if val {
				enabledFlagCount = enabledFlagCount + 1
				enabledFlag = name
			}

			if enabledFlagCount > 1 {
				break
			}
		}

		if enabledFlagCount == 0 {
			return nil, nil
		} else if enabledFlagCount == 1 {
			return enabledFlag, nil
		}

		return nil, errors.New("expected exactly one of boolean flags to be true")
	}
}

func ValueStrat(name string) Strat {
	return func() (interface{}, error) {
		return name, nil
	}
}

func ErrorStrat(err error) Strat {
	return func() (interface{}, error) {
		return nil, err
	}
}
