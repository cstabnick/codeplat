// Package core holds the domain types, the business rules, and the ports
// those rules depend on. It imports no other package in this module.
package core

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid input")
)

type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

func (s Status) Valid() bool {
	return s == StatusTodo || s == StatusInProgress || s == StatusDone
}

// Stage is which agent loop an issue is waiting on. Status says where the
// issue is within that stage, so (stage, todo) is that loop's queue.
type Stage string

const (
	StageRefinement     Stage = "refinement"
	StageImplementation Stage = "implementation"
	StageTesting        Stage = "testing"
)

func (s Stage) Valid() bool {
	return s == StageRefinement || s == StageImplementation || s == StageTesting
}

type Project struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Issue struct {
	ID          string    `json:"id"`
	ProjectKey  string    `json:"project_key"`
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Stage       Stage     `json:"stage"`
	Status      Status    `json:"status"`
	Assignee    string    `json:"assignee"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IssueID formats the Jira-style identifier for an issue.
func IssueID(projectKey string, number int) string {
	return fmt.Sprintf("%s-%d", projectKey, number)
}

type NewProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type NewIssue struct {
	ProjectKey  string `json:"project_key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Assignee    string `json:"assignee"`
	// Stage defaults to refinement.
	Stage Stage `json:"stage,omitempty"`
}

// IssuePatch changes only the fields that are non-nil.
type IssuePatch struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Assignee    *string `json:"assignee,omitempty"`
	Stage       *Stage  `json:"stage,omitempty"`
	Status      *Status `json:"status,omitempty"`
}

type Comment struct {
	IssueID   string    `json:"issue_id"`
	Number    int       `json:"number"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type NewComment struct {
	IssueID string `json:"issue_id"`
	Author  string `json:"author"`
	Body    string `json:"body"`
}

// IssueFilter matches issues on every non-empty field.
type IssueFilter struct {
	ProjectKey string `json:"project_key,omitempty"`
	Stage      Stage  `json:"stage,omitempty"`
	Status     Status `json:"status,omitempty"`
	Assignee   string `json:"assignee,omitempty"`
}

// Store is the persistence port. Implementations return errors wrapping
// ErrNotFound and ErrConflict, and must pass storetest.Run.
type Store interface {
	CreateProject(ctx context.Context, p Project) error
	ListProjects(ctx context.Context) ([]Project, error)

	// CreateIssue allocates the next number in the issue's project and
	// returns the issue with Number and ID set.
	CreateIssue(ctx context.Context, i Issue) (Issue, error)
	GetIssue(ctx context.Context, id string) (Issue, error)
	ListIssues(ctx context.Context, f IssueFilter) ([]Issue, error)

	// UpdateIssue applies fn to the stored issue atomically. If fn returns
	// an error nothing is saved and that error is returned.
	UpdateIssue(ctx context.Context, id string, fn func(*Issue) error) (Issue, error)

	// AddComment allocates the next number on the comment's issue and
	// returns the comment with Number set.
	AddComment(ctx context.Context, c Comment) (Comment, error)
	// ListComments returns an issue's comments oldest first, or ErrNotFound
	// if the issue does not exist.
	ListComments(ctx context.Context, issueID string) ([]Comment, error)
}
