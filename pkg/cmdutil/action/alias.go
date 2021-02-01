package action

import (
	"errors"

	"github.com/cli/cli/internal/config"
	"github.com/rsteube/carapace"
)

func Aliases() (map[string]string, error) {
	if config, err := config.ParseDefaultConfig(); err != nil {
		return nil, errors.New("failed to parse DefaultConfig:" + err.Error())
	} else {
		if aliasCfg, err := config.Aliases(); err != nil {
			return nil, errors.New("failed to load AliasCfg:" + err.Error())
		} else {
			aliases := make(map[string]string)
			for key, value := range aliasCfg.All() {
				aliases[key] = value
			}
			return aliases, nil
		}
	}
}

func ActionAliases() carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		if aliases, err := Aliases(); err != nil {
			return carapace.ActionMessage(err.Error())
		} else {
			values := make([]string, 0)
			for key, value := range aliases {
				values = append(values, key, value)
			}
			return carapace.ActionValuesDescribed(values...)
		}
	})
}
