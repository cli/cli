package api

import (
	"time"
)

var ReleaseFields = []string{
	"url",
	"tarballUrl",
	"zipballUrl",
	"id",
	"tagName",
	"name",
	"description",
	"isDraft",
	"isLatest",
	"isPrerelease",
	"createdAt",
	"publishedAt",
	"author",
	"releaseAssets",
}

type Release struct {
	DatabaseID   int64
	ID           string
	TagName      string
	Name         string
	Description  string
	IsDraft      bool
	IsPrerelease bool
	CreatedAt    time.Time
	PublishedAt  *time.Time
	URL          string
	ResourcePath string
	Author       Author

	ReleaseAssets struct {
		TotalCount int
		Nodes      []ReleaseAsset
	} `graphql:"releaseAssets(first: 100)"`

	Tag struct {
		ID   string
		Name string

		Target struct {
			OID string
			// commitResourcePath string
		}
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
