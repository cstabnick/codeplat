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
	// StatusBlocked marks an issue its assignee cannot finish. It keeps the
	// assignee, so no other worker claims it, and it is in no loop's queue.
	StatusBlocked Status = "blocked"
)

func (s Status) Valid() bool {
	return s == StatusTodo || s == StatusInProgress || s == StatusDone || s == StatusBlocked
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
	ID          string `json:"id"`
	ProjectKey  string `json:"project_key"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Stage       Stage  `json:"stage"`
	Status      Status `json:"status"`
	Assignee    string `json:"assignee"`
	// Parent is the ID of the goal this issue is part of, or "" for none.
	// A goal and its parts are in the same project, and goals do not nest.
	Parent string `json:"parent"`
	// Epic is the number of the epic this issue belongs to within its
	// parent, from 1 upward. Zero means no epic. Issues without a parent
	// count their epics within the project. Epic n waits for every issue
	// with the same project and parent in epics 1 to n-1 to be done.
	Epic int `json:"epic"`
	// DependsOn lists the IDs of issues that must be done before this one
	// starts.
	DependsOn []string  `json:"depends_on"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	// Parent is optional; see Issue.
	Parent string `json:"parent,omitempty"`
	// Epic and DependsOn are optional; see Issue.
	Epic      int      `json:"epic,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// IssuePatch changes only the fields that are non-nil.
type IssuePatch struct {
	Title       *string   `json:"title,omitempty"`
	Description *string   `json:"description,omitempty"`
	Assignee    *string   `json:"assignee,omitempty"`
	Stage       *Stage    `json:"stage,omitempty"`
	Status      *Status   `json:"status,omitempty"`
	Parent      *string   `json:"parent,omitempty"`
	Epic        *int      `json:"epic,omitempty"`
	DependsOn   *[]string `json:"depends_on,omitempty"`
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

// Epic names one epic. Parent is the goal the epic belongs to, or ""
// for the epics of a project's issues that have no parent.
type Epic struct {
	ProjectKey string `json:"project_key"`
	Parent     string `json:"parent"`
	Number     int    `json:"number"`
	Name       string `json:"name"`
}

// IssueFilter matches issues on every non-empty field.
type IssueFilter struct {
	ProjectKey string `json:"project_key,omitempty"`
	Stage      Stage  `json:"stage,omitempty"`
	Status     Status `json:"status,omitempty"`
	Assignee   string `json:"assignee,omitempty"`
	// Parent keeps only the issues whose Parent is this ID.
	Parent string `json:"parent,omitempty"`
	// Ready keeps only issues for which BlockedBy returns nothing. Stores
	// ignore it; Service.ListIssues applies it before cutting the page.
	Ready bool `json:"ready,omitempty"`
	// Page selects one window of the matches. Service.ListIssues fills in
	// the default limit; a store treats Limit 0 as no limit.
	Page
}

type EventType string

const (
	EventProjectCreated EventType = "project.created"
	EventIssueCreated   EventType = "issue.created"
	EventIssueUpdated   EventType = "issue.updated"
	EventCommentAdded   EventType = "comment.added"
	EventEpicSet        EventType = "epic.set"
)

// Event records that something changed. Stores append one for every
// successful change, in the same transaction as the change.
type Event struct {
	ID         int64     `json:"id"`
	Type       EventType `json:"type"`
	ProjectKey string    `json:"project_key"`
	IssueID    string    `json:"issue_id,omitempty"`
	At         time.Time `json:"at"`
}

// Store is the persistence port. Implementations return errors wrapping
// ErrNotFound and ErrConflict, and must pass storetest.Run. Every successful
// CreateProject, CreateIssue, UpdateIssue, AddComment and SetEpic records exactly one Event.
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

	// SetEpic stores e, replacing the name of an epic that already has the
	// same ProjectKey, Parent and Number. It returns ErrNotFound if the
	// project does not exist. It records one Event of type EventEpicSet
	// with e.ProjectKey, IssueID e.Parent and time at.
	SetEpic(ctx context.Context, e Epic, at time.Time) error
	// ListEpics returns the epics of a project ordered by Parent, then
	// Number. An empty projectKey lists every project's epics, ordered by
	// ProjectKey first. It never returns nil.
	ListEpics(ctx context.Context, projectKey string) ([]Epic, error)

	// EventsSince returns the events whose ID is greater than after, oldest
	// first. It never returns nil.
	EventsSince(ctx context.Context, after int64) ([]Event, error)
	// LastEventID returns the ID of the newest event, or 0 if there are none.
	LastEventID(ctx context.Context) (int64, error)
}
