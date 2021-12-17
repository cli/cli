package cmdutil

import (
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NilStringFlag defines a new flag with a string pointer receiver. This is useful for differentiating
// between the flag being set to a blank value and the flag not being passed at all.
func NilStringFlag(cmd *cobra.Command, p **string, name string, shorthand string, usage string) *pflag.Flag {
	return cmd.Flags().VarPF(newStringValue(p), name, shorthand, usage)
}

// NilBoolFlag defines a new flag with a bool pointer receiver. This is useful for differentiating
// between the flag being explicitly set to a false value and the flag not being passed at all.
func NilBoolFlag(cmd *cobra.Command, p **bool, name string, shorthand string, usage string) *pflag.Flag {
	f := cmd.Flags().VarPF(newBoolValue(p), name, shorthand, usage)
	f.NoOptDefVal = "true"
	return f
}

type stringValue struct {
	string **string
}

func newStringValue(p **string) *stringValue {
	return &stringValue{p}
}

func (s *stringValue) Set(value string) error {
	*s.string = &value
	return nil
}

func (s *stringValue) String() string {
	if s.string == nil || *s.string == nil {
		return ""
	}
	return **s.string
}

func (s *stringValue) Type() string {
	return "string"
}

type boolValue struct {
	bool **bool
}

func newBoolValue(p **bool) *boolValue {
	return &boolValue{p}
}

func (b *boolValue) Set(value string) error {
	v, err := strconv.ParseBool(value)
	*b.bool = &v
	return err
}

func (b *boolValue) String() string {
	if b.bool == nil || *b.bool == nil {
		return "false"
	} else if **b.bool {
		return "true"
	}
	return "false"
}

func (b *boolValue) Type() string {
	return "bool"
}

func (b *boolValue) IsBoolFlag() bool {
	return true
}
