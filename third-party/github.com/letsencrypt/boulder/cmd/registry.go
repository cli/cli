package cmd

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/letsencrypt/validator/v10"
)

type ConfigValidator struct {
	Config     interface{}
	Validators map[string]validator.Func
}

var registry struct {
	sync.Mutex
	commands map[string]func()
	configs  map[string]*ConfigValidator
}

// RegisterCommand registers a subcommand and its corresponding config
// validator. The provided func() is called when the subcommand is invoked on
// the command line. The ConfigValidator is optional and used to validate the
// config file for the subcommand.
func RegisterCommand(name string, f func(), cv *ConfigValidator) {
	registry.Lock()
	defer registry.Unlock()

	if registry.commands == nil {
		registry.commands = make(map[string]func())
	}

	if registry.commands[name] != nil {
		panic(fmt.Sprintf("command %q was registered twice", name))
	}
	registry.commands[name] = f

	if cv == nil {
		return
	}

	if registry.configs == nil {
		registry.configs = make(map[string]*ConfigValidator)
	}

	if registry.configs[name] != nil {
		panic(fmt.Sprintf("config validator for command %q was registered twice", name))
	}
	registry.configs[name] = cv
}

func LookupCommand(name string) func() {
	registry.Lock()
	defer registry.Unlock()
	return registry.commands[name]
}

func AvailableCommands() []string {
	registry.Lock()
	defer registry.Unlock()
	var avail []string
	for name := range registry.commands {
		avail = append(avail, name)
	}
	sort.Strings(avail)
	return avail
}

// LookupConfigValidator constructs an instance of the *ConfigValidator for the
// given Boulder component name. If no *ConfigValidator was registered, nil is
// returned.
func LookupConfigValidator(name string) *ConfigValidator {
	registry.Lock()
	defer registry.Unlock()
	if registry.configs[name] == nil {
		return nil
	}

	// Create a new copy of the config struct so that we can validate it
	// multiple times without mutating the registry's copy.
	copy := reflect.New(reflect.ValueOf(
		registry.configs[name].Config).Elem().Type(),
	).Interface()

	return &ConfigValidator{
		Config:     copy,
		Validators: registry.configs[name].Validators,
	}
}

// AvailableConfigValidators returns a list of Boulder component names for which
// a *ConfigValidator has been registered.
func AvailableConfigValidators() []string {
	registry.Lock()
	defer registry.Unlock()
	var avail []string
	for name := range registry.configs {
		avail = append(avail, name)
	}
	sort.Strings(avail)
	return avail
}
