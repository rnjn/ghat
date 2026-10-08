package gh

import (
	"encoding/json"
	"strings"
	"time"
)

// Repo is a repository the user can access.
type Repo struct {
	Owner         string
	Name          string
	PushedAt      time.Time
	Archived      bool
	DefaultBranch string
}

// Key returns "owner/name".
func (r Repo) Key() string { return r.Owner + "/" + r.Name }

func (r *Repo) UnmarshalJSON(b []byte) error {
	var raw struct {
		Name     string                 `json:"name"`
		Owner    struct{ Login string } `json:"owner"`
		PushedAt time.Time              `json:"pushed_at"`
		Archived bool                   `json:"archived"`
		Default  string                 `json:"default_branch"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*r = Repo{Owner: raw.Owner.Login, Name: raw.Name, PushedAt: raw.PushedAt, Archived: raw.Archived, DefaultBranch: raw.Default}
	return nil
}

// Run is a workflow run. Status and Conclusion are GitHub's strings verbatim.
type Run struct {
	ID           int64     `json:"id"`
	RepoKey      string    `json:"repo"`
	Number       int       `json:"number"`
	WorkflowName string    `json:"workflow"`
	WorkflowID   int64     `json:"workflow_id"`
	Branch       string    `json:"branch"`
	Event        string    `json:"event"`
	Actor        string    `json:"actor"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	HTMLURL      string    `json:"html_url"`
	// HeadSHA is the commit the run ran on; CommitMessage is the first line
	// of its message.
	HeadSHA       string `json:"head_sha"`
	CommitMessage string `json:"commit_message"`
	CommitAuthor  string `json:"commit_author"`
}

// ShortSHA is the 7-character commit ID, or "" without a commit.
func (r Run) ShortSHA() string {
	if len(r.HeadSHA) < 7 {
		return r.HeadSHA
	}
	return r.HeadSHA[:7]
}

// CommitURL is the commit's diff page on GitHub, or "" without a commit.
func (r Run) CommitURL() string {
	if r.HeadSHA == "" || r.RepoKey == "" {
		return ""
	}
	return "https://github.com/" + r.RepoKey + "/commit/" + r.HeadSHA
}

func (r *Run) UnmarshalJSON(b []byte) error {
	var raw struct {
		ID         int64                  `json:"id"`
		RunNumber  int                    `json:"run_number"`
		Name       string                 `json:"name"`
		WorkflowID int64                  `json:"workflow_id"`
		HeadBranch string                 `json:"head_branch"`
		Event      string                 `json:"event"`
		Status     string                 `json:"status"`
		Conclusion string                 `json:"conclusion"`
		CreatedAt  time.Time              `json:"created_at"`
		UpdatedAt  time.Time              `json:"updated_at"`
		HTMLURL    string                 `json:"html_url"`
		Actor      struct{ Login string } `json:"actor"`
		HeadSHA    string                 `json:"head_sha"`
		HeadCommit *struct {
			Message string
			Author  struct{ Name string }
		} `json:"head_commit"`
		Repository Repo `json:"repository"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*r = Run{
		ID: raw.ID, Number: raw.RunNumber, WorkflowName: raw.Name, WorkflowID: raw.WorkflowID,
		Branch: raw.HeadBranch, Event: raw.Event, Actor: raw.Actor.Login,
		Status: raw.Status, Conclusion: raw.Conclusion,
		CreatedAt: raw.CreatedAt, UpdatedAt: raw.UpdatedAt, HTMLURL: raw.HTMLURL,
		RepoKey: raw.Repository.Key(), HeadSHA: raw.HeadSHA,
	}
	if c := raw.HeadCommit; c != nil {
		r.CommitMessage, _, _ = strings.Cut(c.Message, "\n")
		r.CommitAuthor = c.Author.Name
	}
	if r.RepoKey == "/" {
		r.RepoKey = ""
	}
	return nil
}

// Job is one job of a run.
type Job struct {
	ID          int64     `json:"id"`
	RunID       int64     `json:"run_id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	HTMLURL     string    `json:"html_url"`
	Steps       []Step    `json:"steps"`
}

// Step is one step of a job.
type Step struct {
	Number      int       `json:"number"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}
