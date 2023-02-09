package ruleset

type RuleType string
type Enforcement string
type MatchingOperator string

const (
	RuleTypeCommitAuthorEmailPattern RuleType = "commit_author_email_pattern"
	RuleTypePullRequest              RuleType = "pull_request"
	// TODO others

	EnforcementEnabled  Enforcement = "enabled"
	EnforcementDisabled Enforcement = "disabled"

	MatchingOperatorStartsWith MatchingOperator = "starts_with"
	MatchingOperatorEndsWith   MatchingOperator = "ends_with"
	MatchingOperatorContains   MatchingOperator = "contains"
	MatchingOperatorRegex      MatchingOperator = "regex"
)

type ConfigurationPullRequest struct {
	DissmissStaleReviewsOnPush   bool
	RequireCodeOwnerReview       bool
	RequestLastPushApproval      bool
	RequiredApprovingReviewCount int
}

type BranchNamePattern struct {
	Name     string
	Negate   bool
	Operator MatchingOperator
}

type Rule struct {
	ID            string
	Type          RuleType
	Enforcement   Enforcement
	Configuration interface{}
}
