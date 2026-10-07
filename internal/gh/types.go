package gh

import (
	"encoding/json"
	"time"
)

// Repo is a repository the user can access.
type Repo struct {
	Owner    string
	Name     string
	PushedAt time.Time
	Archived bool
}

// Key returns "owner/name".
func (r Repo) Key() string { return r.Owner + "/" + r.Name }

func (r *Repo) UnmarshalJSON(b []byte) error {
	var raw struct {
		Name     string                 `json:"name"`
		Owner    struct{ Login string } `json:"owner"`
		PushedAt time.Time              `json:"pushed_at"`
		Archived bool                   `json:"archived"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*r = Repo{Owner: raw.Owner.Login, Name: raw.Name, PushedAt: raw.PushedAt, Archived: raw.Archived}
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
		Repository Repo                   `json:"repository"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*r = Run{
		ID: raw.ID, Number: raw.RunNumber, WorkflowName: raw.Name, WorkflowID: raw.WorkflowID,
		Branch: raw.HeadBranch, Event: raw.Event, Actor: raw.Actor.Login,
		Status: raw.Status, Conclusion: raw.Conclusion,
		CreatedAt: raw.CreatedAt, UpdatedAt: raw.UpdatedAt, HTMLURL: raw.HTMLURL,
		RepoKey: raw.Repository.Key(),
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
