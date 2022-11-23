package cobra

func (c *Command) UsageString() string {
	return "Usage: " + c.UseLine() + "\n"
}

func (c *Command) InitDefaultHelpCmd()  {}
func (c *Command) InitDefaultHelpFlag() {}

func (c *Command) IsAvailableCommand() bool {
	return true
}

func (c *Command) IsAdditionalHelpTopicCommand() bool {
	return true
}

func (c *Command) SetHelpFunc(f func(*Command, []string)) {}

func (c *Command) SetUsageFunc(f func(*Command) error) {}

func (c *Command) SuggestionsFor(typedName string) []string {
	return nil
}
