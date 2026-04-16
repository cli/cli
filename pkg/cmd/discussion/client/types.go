package client

import "time"

// Discussion represents a GitHub Discussion as a domain object.
// Fields carry no JSON tags; serialization is handled by ExportData.
type Discussion struct {
	ID             string
	Number         int
	Title          string
	Body           string
	URL            string
	Closed         bool
	StateReason    string
	Author         DiscussionActor
	Category       DiscussionCategory
	Labels         []DiscussionLabel
	Answered       bool
	AnswerChosenAt time.Time
	AnswerChosenBy *DiscussionActor
	Comments       DiscussionCommentList
	ReactionGroups []ReactionGroup
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ClosedAt       time.Time
	Locked         bool
}

// ExportData returns a map of the requested fields for JSON output.
// Because domain types carry no JSON struct tags, each field is mapped
// explicitly rather than using reflection.
func (d Discussion) ExportData(fields []string) map[string]interface{} {
	data := map[string]interface{}{}
	for _, f := range fields {
		switch f {
		case "id":
			data[f] = d.ID
		case "number":
			data[f] = d.Number
		case "title":
			data[f] = d.Title
		case "body":
			data[f] = d.Body
		case "url":
			data[f] = d.URL
		case "closed":
			data[f] = d.Closed
		case "state":
			if d.Closed {
				data[f] = "CLOSED"
			} else {
				data[f] = "OPEN"
			}
		case "stateReason":
			data[f] = d.StateReason
		case "author":
			data[f] = d.Author.Export()
		case "category":
			data[f] = d.Category.Export()
		case "labels":
			labels := make([]interface{}, len(d.Labels))
			for i, l := range d.Labels {
				labels[i] = l.Export()
			}
			data[f] = labels
		case "answered":
			data[f] = d.Answered
		case "answerChosenAt":
			if d.AnswerChosenAt.IsZero() {
				data[f] = nil
			} else {
				data[f] = d.AnswerChosenAt
			}
		case "answerChosenBy":
			if d.AnswerChosenBy == nil {
				data[f] = nil
			} else {
				data[f] = d.AnswerChosenBy.Export()
			}
		case "comments":
			comments := make([]interface{}, len(d.Comments.Comments))
			for i, c := range d.Comments.Comments {
				comments[i] = c.Export()
			}
			data[f] = map[string]interface{}{
				"totalCount": d.Comments.TotalCount,
				"nodes":      comments,
			}
		case "reactionGroups":
			reactions := make([]interface{}, len(d.ReactionGroups))
			for i, rg := range d.ReactionGroups {
				reactions[i] = rg.Export()
			}
			data[f] = reactions
		case "createdAt":
			data[f] = d.CreatedAt
		case "updatedAt":
			data[f] = d.UpdatedAt
		case "closedAt":
			if d.ClosedAt.IsZero() {
				data[f] = nil
			} else {
				data[f] = d.ClosedAt
			}
		case "locked":
			data[f] = d.Locked
		}
	}
	return data
}

// DiscussionActor represents a GitHub actor (user or bot) associated with a discussion.
type DiscussionActor struct {
	ID    string
	Login string
	Name  string
}

// Export returns the author as a map for JSON output.
func (a DiscussionActor) Export() map[string]interface{} {
	return map[string]interface{}{
		"id":    a.ID,
		"login": a.Login,
		"name":  a.Name,
	}
}

// DiscussionCategory represents a discussion category within a repository.
type DiscussionCategory struct {
	ID           string
	Name         string
	Slug         string
	Emoji        string
	IsAnswerable bool
}

// Export returns the category as a map for JSON output.
func (c DiscussionCategory) Export() map[string]interface{} {
	return map[string]interface{}{
		"id":           c.ID,
		"name":         c.Name,
		"slug":         c.Slug,
		"emoji":        c.Emoji,
		"isAnswerable": c.IsAnswerable,
	}
}

// DiscussionLabel represents a label applied to a discussion.
type DiscussionLabel struct {
	ID    string
	Name  string
	Color string
}

// Export returns the label as a map for JSON output.
func (l DiscussionLabel) Export() map[string]interface{} {
	return map[string]interface{}{
		"id":    l.ID,
		"name":  l.Name,
		"color": l.Color,
	}
}

// DiscussionComment represents a comment or reply on a discussion.
type DiscussionComment struct {
	ID             string
	URL            string
	Author         DiscussionActor
	Body           string
	CreatedAt      time.Time
	IsAnswer       bool
	UpvoteCount    int
	ReactionGroups []ReactionGroup
	Replies        []DiscussionComment
	TotalReplies   int
}

// Export returns the comment as a map for JSON output.
func (c DiscussionComment) Export() map[string]interface{} {
	replies := make([]interface{}, len(c.Replies))
	for i, r := range c.Replies {
		replies[i] = r.Export()
	}
	reactions := make([]interface{}, len(c.ReactionGroups))
	for i, rg := range c.ReactionGroups {
		reactions[i] = rg.Export()
	}
	return map[string]interface{}{
		"id":             c.ID,
		"url":            c.URL,
		"author":         c.Author.Export(),
		"body":           c.Body,
		"createdAt":      c.CreatedAt,
		"isAnswer":       c.IsAnswer,
		"upvoteCount":    c.UpvoteCount,
		"reactionGroups": reactions,
		"replies":        replies,
		"totalReplies":   c.TotalReplies,
	}
}

// DiscussionCommentList represents a paginated list of comments on a discussion.
type DiscussionCommentList struct {
	Comments   []DiscussionComment
	TotalCount int
}

// ReactionGroup represents a set of reactions of the same type.
type ReactionGroup struct {
	Content    string
	TotalCount int
}

// Export returns the reaction group as a map for JSON output.
func (rg ReactionGroup) Export() map[string]interface{} {
	return map[string]interface{}{
		"content":    rg.Content,
		"totalCount": rg.TotalCount,
	}
}

// CloseReason represents the reason for closing a discussion.
type CloseReason string

const (
	// CloseReasonResolved indicates the discussion topic has been resolved.
	CloseReasonResolved CloseReason = "RESOLVED"
	// CloseReasonOutdated indicates the discussion is no longer relevant.
	CloseReasonOutdated CloseReason = "OUTDATED"
	// CloseReasonDuplicate indicates the discussion is a duplicate of another.
	CloseReasonDuplicate CloseReason = "DUPLICATE"
)

// Domain-level filter constants for state.
const (
	FilterStateOpen   = "open"
	FilterStateClosed = "closed"
)

// Domain-level constants for order-by field.
const (
	OrderByCreated = "created"
	OrderByUpdated = "updated"
)

// Domain-level constants for order direction.
const (
	OrderDirectionAsc  = "asc"
	OrderDirectionDesc = "desc"
)

// DiscussionListResult holds the result of a List or Search call,
// including the discussions, total count, and pagination cursor.
type DiscussionListResult struct {
	Discussions []Discussion
	TotalCount  int
	NextCursor  string
}

// ListFilters holds parameters for the repository.discussions query.
// CategoryID must be resolved by the caller before passing to List.
// A nil State indicates no state filtering (all states).
type ListFilters struct {
	State      *string
	CategoryID string
	Answered   *bool
	OrderBy    string
	Direction  string
}

// SearchFilters holds parameters for the search query used when
// author or label filtering is required.
// A nil State indicates no state filtering (all states).
type SearchFilters struct {
	Author    string
	Labels    []string
	State     *string
	Category  string
	Answered  *bool
	Keywords  string
	OrderBy   string
	Direction string
}

// CreateDiscussionInput holds the parameters for creating a discussion.
type CreateDiscussionInput struct {
	RepositoryID string
	CategoryID   string
	Title        string
	Body         string
}

// UpdateDiscussionInput holds optional parameters for updating a discussion.
// Nil pointer fields are left unchanged.
type UpdateDiscussionInput struct {
	DiscussionID string
	Title        *string
	Body         *string
	CategoryID   *string
}
