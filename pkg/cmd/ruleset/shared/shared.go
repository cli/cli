package shared

import (
	"fmt"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

type RulesetGraphQL struct {
	DatabaseId  int
	Name        string
	Target      string
	Enforcement string
	Source      struct {
		TypeName string `json:"__typename"`
		Owner    string
	}
	Rules struct {
		TotalCount int
	}
}

type RulesetREST struct {
	Id           int
	Name         string
	Target       string
	Enforcement  string
	BypassActors []struct {
		ActorId    int    `json:"actor_id"`
		ActorType  string `json:"actor_type"`
		BypassMode string `json:"bypass_mode"`
	} `json:"bypass_actors"`
	Conditions map[string]map[string]interface{}
	// TODO is this source field used?
	SourceType string `json:"source_type"`
	Source     string
	Rules      []struct {
		Type       string
		Parameters map[string]interface{}
	}
	Links struct {
		Html struct {
			Href string
		}
	} `json:"_links"`
}

// Returns the source of the ruleset in the format "owner/name (repo)" or "owner (org)"
func RulesetSource(rs RulesetGraphQL) string {
	var level string
	if rs.Source.TypeName == "Repository" {
		level = "repo"
	} else if rs.Source.TypeName == "Organization" {
		level = "org"
	} else {
		level = "unknown"
	}

	return fmt.Sprintf("%s (%s)", rs.Source.Owner, level)
}

func NoRulesetsFoundError(orgOption string, repoI ghrepo.Interface, includeParents bool) error {
	entityName := EntityName(orgOption, repoI)
	parentsMsg := ""
	if includeParents {
		parentsMsg = " or its parents"
	}
	return cmdutil.NewNoResultsError(fmt.Sprintf("no rulesets found in %s%s", entityName, parentsMsg))
}

func EntityName(orgOption string, repoI ghrepo.Interface) string {
	if orgOption != "" {
		return orgOption
	}
	return ghrepo.FullName(repoI)
}
