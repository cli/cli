package cobra

import "github.com/spf13/pflag"

func (c *Command) ArgsLenAtDash() int {
	return 0
}

func (c *Command) Flags() *pflag.FlagSet {
	if c.localFlags == nil {
		flags := pflag.NewFlagSet(c.CommandPath(), pflag.ContinueOnError)
		flags.SetOutput(c.ErrOrStderr())
		c.localFlags = flags
	}
	return c.localFlags
}

func (c *Command) LocalFlags() *pflag.FlagSet {
	return c.Flags()
}

func (c *Command) PersistentFlags() *pflag.FlagSet {
	return pflag.NewFlagSet("", pflag.ContinueOnError)
}

func (c *Command) InheritedFlags() *pflag.FlagSet {
	return pflag.NewFlagSet("", pflag.ContinueOnError)
}

func (c *Command) NonInheritedFlags() *pflag.FlagSet {
	return c.Flags()
}

func (c *Command) FlagErrorFunc() func(*Command, error) error {
	return nil
}

func (c *Command) SetFlagErrorFunc(f func(*Command, error) error) {}
