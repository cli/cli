package client

import (
	"slices"
	"time"
)

// The artifact types gh knows. The API stores a type with each artifact, and
// gh treats generic and plan artifacts alike as documents.
const (
	TypeGeneric = "generic"
	TypePlan    = "plan"
	TypeLink    = "link"
)

// ArtifactFields lists the --json fields of an artifact. They are the fields
// the API returns, camelCased the way gh release names REST fields.
var ArtifactFields = []string{
	"body",
	"bodyHtml",
	"createdAt",
	"creator",
	"description",
	"id",
	"name",
	"number",
	"type",
	"updatedAt",
	"updatedByActor",
}

// ArtifactWithVersionsFields lists the --json fields of a single artifact: the
// artifact fields, plus versions. List responses have no edit history, so only
// commands that get one artifact offer versions.
var ArtifactWithVersionsFields = append(slices.Clone(ArtifactFields), "versions")

// Artifact is an issue artifact as the REST API returns it. The API can return
// null for the users and timestamps, so those are pointers.
type Artifact struct {
	// ID is the API's global artifact ID. Commands address an artifact by its
	// issue and Number instead.
	ID             int64      `json:"id"`
	Number         int        `json:"number"`
	Type           string     `json:"type"`
	Name           string     `json:"name"`
	Body           string     `json:"body"`
	BodyHTML       string     `json:"body_html"`
	Description    string     `json:"description"`
	Creator        *Actor     `json:"creator"`
	UpdatedByActor *Actor     `json:"updated_by_actor"`
	CreatedAt      *time.Time `json:"created_at"`
	UpdatedAt      *time.Time `json:"updated_at"`
}

// Actor is a user who created or updated an artifact, or saved one of its
// versions.
type Actor struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// UploadTarget is what a command that uploads files with --attach learns
// about an issue and its repository, from one lookup.
type UploadTarget struct {
	// IsPullRequest reports whether the number is a pull request rather than
	// an issue.
	IsPullRequest bool
	// RepositoryID is the repository's REST ID, which uploads are made
	// against, and ViewerPermission is the viewer's permission on it, such as
	// WRITE. attachments.NewUploader checks both.
	RepositoryID     int64
	ViewerPermission string
}

// ArtifactWithVersions is one artifact with its edit history, as the API
// returns a single artifact.
type ArtifactWithVersions struct {
	Artifact
	// Versions is the edit history as the API returns it: newest first,
	// including the current version.
	Versions []Version
}

// Version is one saved version of an artifact. The API can return null for
// the actor and the timestamp, so those are pointers.
type Version struct {
	Version   int        `json:"version"`
	Name      string     `json:"name"`
	Body      string     `json:"body"`
	BodyHTML  string     `json:"body_html"`
	CreatedAt *time.Time `json:"created_at"`
	// Actor is the user who saved this version.
	Actor *Actor `json:"actor"`
}

// ExportData returns the requested fields for --json output. Fields the API
// returned as null stay null.
func (a Artifact) ExportData(fields []string) map[string]any {
	data := map[string]any{}
	for _, f := range fields {
		switch f {
		case "body":
			data[f] = a.Body
		case "bodyHtml":
			data[f] = a.BodyHTML
		case "createdAt":
			data[f] = a.CreatedAt
		case "creator":
			data[f] = a.Creator.export()
		case "description":
			data[f] = a.Description
		case "id":
			data[f] = a.ID
		case "name":
			data[f] = a.Name
		case "number":
			data[f] = a.Number
		case "type":
			data[f] = a.Type
		case "updatedAt":
			data[f] = a.UpdatedAt
		case "updatedByActor":
			data[f] = a.UpdatedByActor.export()
		}
	}
	return data
}

// ExportData returns the requested fields for --json output, where versions
// is the edit history, newest first. Fields the API returned as null stay
// null.
func (a ArtifactWithVersions) ExportData(fields []string) map[string]any {
	data := a.Artifact.ExportData(fields)
	if slices.Contains(fields, "versions") {
		versions := make([]map[string]any, 0, len(a.Versions))
		for _, v := range a.Versions {
			versions = append(versions, v.export())
		}
		data["versions"] = versions
	}
	return data
}

func (v Version) export() map[string]any {
	return map[string]any{
		"version":   v.Version,
		"name":      v.Name,
		"body":      v.Body,
		"bodyHtml":  v.BodyHTML,
		"createdAt": v.CreatedAt,
		"actor":     v.Actor.export(),
	}
}

func (a *Actor) export() any {
	if a == nil {
		return nil
	}
	return map[string]any{
		"id":    a.ID,
		"login": a.Login,
	}
}
