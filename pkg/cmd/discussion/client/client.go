// Package client provides an abstraction layer for interacting with the
// GitHub Discussions GraphQL API. The DiscussionClient interface defines all
// supported operations and can be replaced with a mock in tests.
package client

import "github.com/cli/cli/v2/internal/ghrepo"

//go:generate moq -rm -out client_mock.go . DiscussionClient

// DiscussionClient defines operations for interacting with the GitHub Discussions API.
type DiscussionClient interface {
	List(repo ghrepo.Interface, filters ListFilters, after string, limit int) (*DiscussionListResult, error)
	Search(repo ghrepo.Interface, filters SearchFilters, after string, limit int) (*DiscussionListResult, error)
	GetByNumber(repo ghrepo.Interface, number int) (*Discussion, error)
	GetWithComments(repo ghrepo.Interface, number int, commentLimit int, after string, newest bool) (*Discussion, error)
	GetCommentReplies(repo ghrepo.Interface, number int, commentID string, limit int, after string, newest bool) (*Discussion, error)
	ListCategories(repo ghrepo.Interface) ([]DiscussionCategory, error)
	ListLabels(repo ghrepo.Interface) ([]DiscussionLabel, error)
	Create(repo ghrepo.Interface, input CreateDiscussionInput) (*Discussion, error)
	Update(repo ghrepo.Interface, input UpdateDiscussionInput) (*Discussion, error)
	Close(repo ghrepo.Interface, id string, reason CloseReason) (*Discussion, error)
	Reopen(repo ghrepo.Interface, id string) (*Discussion, error)
	AddComment(repo ghrepo.Interface, discussionID string, body string, replyToID string) (*DiscussionComment, error)
	Lock(repo ghrepo.Interface, id string, reason string) error
	Unlock(repo ghrepo.Interface, id string) error
	MarkAnswer(repo ghrepo.Interface, commentID string) error
	UnmarkAnswer(repo ghrepo.Interface, commentID string) error
}
