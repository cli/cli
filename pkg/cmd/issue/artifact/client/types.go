package client

import "time"

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

// Actor is the user who created or last updated an artifact.
type Actor struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
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
			data[f] = exportTime(a.CreatedAt)
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
			data[f] = exportTime(a.UpdatedAt)
		case "updatedByActor":
			data[f] = a.UpdatedByActor.export()
		}
	}
	return data
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

func exportTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
