package shared

type RulesetGraphQL struct {
	DatabaseId  int
	Name        string
	Target      string
	Enforcement string
	Source      struct {
		RepoOwner string
		OrgOwner  string
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
	BypassMode   string `json:"bypass_mode"`
	BypassActors []struct {
		ActorId   int    `json:"actor_id"`
		ActorType string `json:"actor_type"`
	} `json:"bypass_actors"`
	Conditions map[string]map[string]interface{}
	// TODO is this source field used?
	SourceType string `json:"source_type"`
	Source     string
	Rules      []struct{}
}
