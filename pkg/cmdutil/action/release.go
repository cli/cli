package action

import (
	"time"

	"github.com/cli/cli/utils"
	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type release struct {
	Name         string
	IsPrerelease bool
	CreatedAt    time.Time
	Tag          struct {
		Name string
	}
	ReleaseAssets struct {
		Nodes []struct {
			Name string
			Size int
		}
	}
}

type releaseQuery struct {
	Data struct {
		Repository struct {
			Releases struct {
				Edges []struct {
					Node release
				}
			}
		}
	}
}

func ActionReleases(cmd *cobra.Command) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult releaseQuery
		return GraphQlAction(cmd, `repository(owner: $owner, name: $repo) { releases(first: 100, orderBy: {direction: DESC, field: CREATED_AT}) { edges { node { createdAt isPrerelease name tag { name } } } } }`, &queryResult, func() carapace.Action {
			releases := queryResult.Data.Repository.Releases.Edges
			vals := make([]string, 0, len(releases)*2)
			for _, release := range releases {
				if release.Node.Tag.Name != "" {
					vals = append(vals, release.Node.Tag.Name, utils.FuzzyAgo(time.Since(release.Node.CreatedAt)))
				}
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}

func ActionReleaseAssets(cmd *cobra.Command, tag string) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult releaseQuery
		return GraphQlAction(cmd, `repository(owner: $owner, name: $repo) { releases(first: 100, orderBy: {direction: DESC, field: CREATED_AT}) { edges { node { createdAt isPrerelease name tag { name } releaseAssets(first: 100) { nodes { name size } } } } } }`, &queryResult, func() carapace.Action {
			releases := queryResult.Data.Repository.Releases.Edges
			vals := make([]string, 0)
			for _, release := range releases {
				if release.Node.Tag.Name == tag {
					for _, asset := range release.Node.ReleaseAssets.Nodes {
						vals = append(vals, asset.Name, byteCountSI(asset.Size))
					}
					break
				}
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}
