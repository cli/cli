package api

import (
	"time"
)

type Release struct {
	DatabaseID   int64
	ID           string
	TagName      string
	Name         string
	Body         string
	IsDraft      bool
	IsPrerelease bool
	CreatedAt    time.Time
	PublishedAt  *time.Time
	URL          string
	ResourcePath string
	Assets       []ReleaseAsset
	Author       Author

	Tag struct {
		ID                 string
		Name               string
		Message            string
		OID                string
		CommitResourcePath string
		CommitUrl          string
		Tagger             Author
	}

	TagCommit struct {
		ID         string
		TarballURL string
		ZipballURL string
	}
}

type ReleaseAsset struct {
	ID            string
	Name          string
	Size          int64
	URL           string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DownloadCount int
	ContentType   string
	DownloadURL   string
	UploadedBy    Author
}
