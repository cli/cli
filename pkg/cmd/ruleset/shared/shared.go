package shared

type Ruleset struct {
	Id           int
	Name         string
	Target       string
	Enforcement  string
	BypassMode   string `json:"bypass_mode"`
	BypassActors []struct {
		ActorId   int    `json:"actor_id"`
		ActorType string `json:"actor_type"`
	} `json:"bypass_actors"`
	Conditions map[string]map[string]interface {
		// RefName struct {
		// 	Include []string
		// 	Exclude []string
		// } `json:"ref_name"`
		// RepositoryName struct {
		// 	Include   []string
		// 	Exclude   []string
		// 	Protected bool
		// } `json:"repository_name"`
	}
	Source struct {
		RepoOwner string
		OrgOwner  string
	}
	RulesGql struct {
		TotalCount int
	}
	Rules []interface{}
}
