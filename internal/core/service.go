package core

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var projectKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

// Service implements every use case. Transports call it and nothing else.
type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store, now func() time.Time) *Service {
	return &Service{store: store, now: now}
}

func (s *Service) CreateProject(ctx context.Context, in NewProject) (Project, error) {
	p := Project{
		Key:       normalizeKey(in.Key),
		Name:      strings.TrimSpace(in.Name),
		CreatedAt: s.now().UTC(),
	}
	if !projectKeyPattern.MatchString(p.Key) {
		return Project{}, invalid("project key must be 2-10 letters or digits starting with a letter")
	}
	if p.Name == "" {
		return Project{}, invalid("project name is required")
	}
	if err := s.store.CreateProject(ctx, p); err != nil {
		return Project{}, err
	}
	return p, nil
}

func (s *Service) ListProjects(ctx context.Context) ([]Project, error) {
	return s.store.ListProjects(ctx)
}

func (s *Service) CreateIssue(ctx context.Context, in NewIssue) (Issue, error) {
	now := s.now().UTC()
	i := Issue{
		ProjectKey:  normalizeKey(in.ProjectKey),
		Title:       strings.TrimSpace(in.Title),
		Description: in.Description,
		Stage:       in.Stage,
		Status:      StatusTodo,
		Assignee:    strings.TrimSpace(in.Assignee),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if i.ProjectKey == "" {
		return Issue{}, invalid("project key is required")
	}
	if i.Title == "" {
		return Issue{}, invalid("title is required")
	}
	if i.Stage == "" {
		i.Stage = StageRefinement
	}
	if !i.Stage.Valid() {
		return Issue{}, invalidStage(i.Stage)
	}
	return s.store.CreateIssue(ctx, i)
}

func (s *Service) GetIssue(ctx context.Context, id string) (Issue, error) {
	return s.store.GetIssue(ctx, normalizeKey(id))
}

func (s *Service) ListIssues(ctx context.Context, f IssueFilter) ([]Issue, error) {
	f.ProjectKey = normalizeKey(f.ProjectKey)
	if f.Stage != "" && !f.Stage.Valid() {
		return nil, invalidStage(f.Stage)
	}
	if f.Status != "" && !f.Status.Valid() {
		return nil, invalidStatus(f.Status)
	}
	return s.store.ListIssues(ctx, f)
}

func (s *Service) UpdateIssue(ctx context.Context, id string, patch IssuePatch) (Issue, error) {
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return Issue{}, invalid("title cannot be empty")
		}
		patch.Title = &title
	}
	if patch.Stage != nil && !patch.Stage.Valid() {
		return Issue{}, invalidStage(*patch.Stage)
	}
	if patch.Status != nil && !patch.Status.Valid() {
		return Issue{}, invalidStatus(*patch.Status)
	}
	return s.store.UpdateIssue(ctx, normalizeKey(id), func(i *Issue) error {
		if patch.Title != nil {
			i.Title = *patch.Title
		}
		if patch.Description != nil {
			i.Description = *patch.Description
		}
		if patch.Assignee != nil {
			i.Assignee = strings.TrimSpace(*patch.Assignee)
		}
		if patch.Stage != nil {
			i.Stage = *patch.Stage
		}
		if patch.Status != nil {
			i.Status = *patch.Status
		}
		i.UpdatedAt = s.now().UTC()
		return nil
	})
}

// ClaimIssue takes a todo issue for assignee and marks it in progress. It
// fails with ErrConflict if the issue is not todo, so of several workers
// racing for one issue exactly one wins.
func (s *Service) ClaimIssue(ctx context.Context, id, assignee string) (Issue, error) {
	assignee = strings.TrimSpace(assignee)
	if assignee == "" {
		return Issue{}, invalid("assignee is required to claim an issue")
	}
	return s.store.UpdateIssue(ctx, normalizeKey(id), func(i *Issue) error {
		if i.Status != StatusTodo {
			return fmt.Errorf("issue %s is %s, not todo: %w", i.ID, i.Status, ErrConflict)
		}
		i.Status = StatusInProgress
		i.Assignee = assignee
		i.UpdatedAt = s.now().UTC()
		return nil
	})
}

func (s *Service) AddComment(ctx context.Context, in NewComment) (Comment, error) {
	c := Comment{
		IssueID:   normalizeKey(in.IssueID),
		Author:    strings.TrimSpace(in.Author),
		Body:      strings.TrimSpace(in.Body),
		CreatedAt: s.now().UTC(),
	}
	if c.Body == "" {
		return Comment{}, invalid("comment body is required")
	}
	return s.store.AddComment(ctx, c)
}

func (s *Service) ListComments(ctx context.Context, issueID string) ([]Comment, error) {
	return s.store.ListComments(ctx, normalizeKey(issueID))
}

func normalizeKey(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func invalid(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, msg)
}

func invalidStage(s Stage) error {
	return invalid(fmt.Sprintf("stage %q must be one of %s, %s, %s", s, StageRefinement, StageImplementation, StageTesting))
}

func invalidStatus(s Status) error {
	return invalid(fmt.Sprintf("status %q must be one of %s, %s, %s", s, StatusTodo, StatusInProgress, StatusDone))
}
