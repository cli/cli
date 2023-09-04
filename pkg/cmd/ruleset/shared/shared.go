package shared

import (
	"fmt"
	"sort"
	"strings"

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
	Id                   int
	Name                 string
	Target               string
	Enforcement          string
	CurrentUserCanBypass string `json:"current_user_can_bypass"`
	BypassActors         []struct {
		ActorId    int    `json:"actor_id"`
		ActorType  string `json:"actor_type"`
		BypassMode string `json:"bypass_mode"`
	} `json:"bypass_actors"`
	Conditions map[string]map[string]interface{}
	SourceType string `json:"source_type"`
	Source     string
	Rules      []RulesetRule
	Links      struct {
		Html struct {
			Href string
		}
	} `json:"_links"`
}

type RulesetRule struct {
	Type              string
	Parameters        map[string]interface{}
	RulesetSourceType string `json:"ruleset_source_type"`
	RulesetSource     string `json:"ruleset_source"`
	RulesetId         int    `json:"ruleset_id"`
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

func ParseRulesForDisplay(rules []RulesetRule) string {
	var display strings.Builder

	// sort keys for consistent responses
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].Type < rules[j].Type
	})

	for _, rule := range rules {
		display.WriteString(fmt.Sprintf("- %s", rule.Type))

		if rule.Parameters != nil && len(rule.Parameters) > 0 {
			display.WriteString(": ")

			// sort these keys too for consistency
			params := make([]string, 0, len(rule.Parameters))
			for p := range rule.Parameters {
				params = append(params, p)
			}
			sort.Strings(params)

			for _, n := range params {
				display.WriteString(fmt.Sprintf("[%s: %v] ", n, rule.Parameters[n]))
			}
		}

		// ruleset source info is only returned from the "get rules for a branch" endpoint
		if rule.RulesetSource != "" {
			display.WriteString(
				fmt.Sprintf(
					"\n  (configured in ruleset %d from %s %s)\n",
					rule.RulesetId,
					strings.ToLower(rule.RulesetSourceType),
					rule.RulesetSource,
				),
			)
		}

		display.WriteString("\n")
	}

	return display.String()
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
