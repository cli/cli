package evidence

type sessionManifest struct {
	SchemaVersion int           `json:"schemaVersion"`
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Source        string        `json:"source"`
	Workspace     string        `json:"workspace"`
	Cases         []sessionCase `json:"cases"`
	Output        struct {
		Formats               []string `json:"formats,omitempty"`
		Timing                string   `json:"timing,omitempty"`
		Captions              *bool    `json:"captions,omitempty"`
		MinimumChapterSeconds *float64 `json:"minimumChapterSeconds,omitempty"`
	} `json:"output"`
}

type sessionCase struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Runs  []sessionRun `json:"runs"`
}

type sessionRun struct {
	ID           string `json:"id"`
	Phase        string `json:"phase"`
	Title        string `json:"title"`
	RunDirectory string `json:"runDirectory"`
	Verification string `json:"verification,omitempty"`
}

type chapterPresentation struct {
	Label                  string  `json:"label"`
	Phase                  string  `json:"phase"`
	Status                 string  `json:"status"`
	Title                  string  `json:"title"`
	Command                string  `json:"command"`
	MinimumDurationSeconds float64 `json:"minimumDurationSeconds"`
}

type sessionChapter struct {
	Kind         string  `json:"kind,omitempty"`
	CaseID       string  `json:"caseId"`
	CaseTitle    string  `json:"caseTitle"`
	RunID        string  `json:"runId"`
	Phase        string  `json:"phase"`
	Title        string  `json:"title"`
	RunDirectory string  `json:"runDirectory"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
	Evidence     report  `json:"evidence"`
	Error        string  `json:"error,omitempty"`
}

type sessionCaseResult struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Mode   string `json:"mode"`
}

type sessionOverview struct {
	Mode            string  `json:"mode"`
	Title           string  `json:"title"`
	Pages           int     `json:"pages"`
	SecondsPerPage  int     `json:"secondsPerPage"`
	DurationSeconds float64 `json:"durationSeconds"`
}

type sessionReport struct {
	SchemaVersion  int                 `json:"schemaVersion"`
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	ManifestSHA256 string              `json:"manifestSha256"`
	SessionStatus  string              `json:"sessionStatus"`
	Mode           string              `json:"mode"`
	OrderVerified  bool                `json:"executionOrderVerified"`
	Overview       *sessionOverview    `json:"overview,omitempty"`
	Cases          []sessionCaseResult `json:"cases"`
	Chapters       []sessionChapter    `json:"chapters"`
	Rendering      rendering           `json:"rendering"`
	Report         string              `json:"report,omitempty"`
}
