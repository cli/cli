package action

import (
	"fmt"
	"strings"

	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type gist struct {
	Name        string
	Description string
	Files       []struct {
		Name string
		Size int
	}
}

type gistQuery struct {
	Data struct {
		User struct {
			Gists struct {
				Edges []struct {
					Node gist
				}
			}
		}
	}
}

func ActionGistUrls(cmd *cobra.Command) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		prefix := "https://gist."
		if !strings.HasPrefix(c.CallbackValue, prefix) {
			// ActionMultiParts to force nospace (should maybe added as function to InvokedAction to prevent space suffix)
			return carapace.ActionMultiParts(".", func(c carapace.Context) carapace.Action {
				switch len(c.Parts) {
				case 0:
					return carapace.ActionValues(prefix)
				default:
					return carapace.ActionValues()
				}
			})
		} else {
			c.CallbackValue = strings.TrimPrefix(c.CallbackValue, prefix)
			return carapace.ActionMultiParts("/", func(c carapace.Context) carapace.Action {
				switch len(c.Parts) {
				case 0:
					return ActionConfigHosts().Invoke(c).Suffix("/").ToA()
				case 1:
					return ActionUsers(cmd, UserOpts{Users: true}).Invoke(c).Suffix("/").ToA()
				case 2:
					cmd.Flags().String("repo", fmt.Sprintf("%v/%v/", c.Parts[0], c.Parts[1]), "fake repo flag")
					cmd.Flag("repo").Changed = true
					return ActionGistIds(cmd)
				default:
					return carapace.ActionValues()
				}
			}).Invoke(c).Prefix(prefix).ToA()
		}
	})
}

func ActionGistIds(cmd *cobra.Command) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		user := "$owner"
		if cmd.Flag("repo") == nil || !cmd.Flag("repo").Changed {
			if u, err := defaultUser(); err != nil {
				return carapace.ActionMessage(err.Error())
			} else {
				user = fmt.Sprintf(`"%v"`, u)
			}
		}

		var queryResult gistQuery
		return GraphQlAction(cmd, fmt.Sprintf(`user(login: %v) { gists(first:100, privacy:ALL, orderBy: {field: UPDATED_AT, direction: DESC}) { edges { node { name, description } } } }`, user), &queryResult, func() carapace.Action {
			gists := queryResult.Data.User.Gists.Edges
			vals := make([]string, len(gists)*2)
			for index, gist := range gists {
				vals[index*2] = gist.Node.Name
				vals[index*2+1] = gist.Node.Description
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}

func ActionGists(cmd *cobra.Command) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		if strings.HasPrefix(c.CallbackValue, "h") {
			return ActionGistUrls(cmd)
		} else {
			return ActionGistIds(cmd)
		}
	})
}

func ActionGistFiles(cmd *cobra.Command, name string) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		splitted := strings.Split(name, "/")
		gistName := splitted[len(splitted)-1]
		switch len(splitted) {
		case 1:
		case 5:
			cmd.Flags().String("repo", fmt.Sprintf("%v/%v/", splitted[2], splitted[3]), "fake repo flag")
			cmd.Flag("repo").Changed = true
		default:
			return carapace.ActionMessage("invalid gist id/url: " + name)
		}

		user := "$owner"
		if cmd.Flag("repo") == nil || !cmd.Flag("repo").Changed {
			if u, err := defaultUser(); err != nil {
				return carapace.ActionMessage(err.Error())
			} else {
				user = fmt.Sprintf(`"%v"`, u)
			}
		}

		// TODO query/filter specific gist by name (this is slow)
		var queryResult gistQuery
		return GraphQlAction(cmd, fmt.Sprintf(`user(login: %v) { gists(first:100, privacy:ALL, orderBy: {field: UPDATED_AT, direction: DESC}) { edges { node { name, description, files { name, size } } } } }`, user), &queryResult, func() carapace.Action {
			gists := queryResult.Data.User.Gists.Edges
			vals := make([]string, 0)
			for _, gist := range gists {
				if gist.Node.Name == gistName {
					for _, file := range gist.Node.Files {
						vals = append(vals, file.Name, byteCountSI(file.Size))
					}

				}
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}
