package stack

import "io"

// Workflow submits pushed bookmarks as a chain of pull requests.
type Workflow struct {
	Runner   Runner
	Remote   string
	Bookmark string
	Base     string
	Repo     string
	Draft    bool
	DryRun   bool
	Output   io.Writer
}

type layer struct {
	ID    string
	Head  string
	Title string
}

type baseChange struct {
	Head   string
	Number int
	URL    string
	Base   string
}

type jjLayer struct {
	ID        string       `json:"id"`
	Bookmarks []jjBookmark `json:"bookmarks"`
	Title     string       `json:"title"`
}

type jjBookmark struct {
	Name   string `json:"name"`
	Remote string `json:"remote"`
}

type pullRequest struct {
	Number      int    `json:"number"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
	State       string `json:"state"`
	MergedAt    string `json:"mergedAt"`
	URL         string `json:"url"`
}

type stackMembership struct {
	Number int `json:"number"`
}

type stackResource struct {
	Number       int       `json:"number"`
	PullRequests []stackPR `json:"pull_requests"`
}

type stackPR struct {
	Number   int     `json:"number"`
	MergedAt *string `json:"merged_at"`
}

type stackAction uint8

const (
	stackNoop stackAction = iota
	stackAppend
	stackRestructure
)

type stackPlan struct {
	Action stackAction
	Delta  []int
}
