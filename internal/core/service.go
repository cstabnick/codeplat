package core

import (
	"context"
	"errors"
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
	assignee, err := ownName(ctx, strings.TrimSpace(in.Assignee))
	if err != nil {
		return Issue{}, err
	}
	i := Issue{
		ProjectKey:  normalizeKey(in.ProjectKey),
		Title:       strings.TrimSpace(in.Title),
		Description: in.Description,
		Stage:       in.Stage,
		Status:      StatusTodo,
		Assignee:    assignee,
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
	if in.Epic < 0 {
		return Issue{}, invalid("epic must be 0 or a positive number")
	}
	deps, err := s.checkDependencies(ctx, "", in.DependsOn)
	if err != nil {
		return Issue{}, err
	}
	i.Epic = in.Epic
	i.DependsOn = deps
	return s.store.CreateIssue(ctx, i)
}

// checkDependencies normalizes ids (trim, then normalizeKey) and drops
// duplicates, keeping the first occurrence. It returns a non-nil slice:
// an empty input gives an empty slice. self is the normalized ID of the
// issue being updated, or "" when the issue is new. It returns an invalid
// error (wrapping ErrInvalid) when:
//   - an ID is empty;
//   - an ID equals self;
//   - an ID is not a stored issue (GetIssue returns ErrNotFound);
//   - self is not "" and following DependsOn from the IDs reaches self.
//
// The message names the offending ID.
func (s *Service) checkDependencies(ctx context.Context, self string, ids []string) ([]string, error) {
	deps := []string{}
	seen := make(map[string]bool, len(ids))
	for _, raw := range ids {
		id := normalizeKey(raw)
		if id == "" {
			return nil, invalid("dependency ID is empty")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if id == self {
			return nil, invalid(fmt.Sprintf("issue %s cannot depend on itself", id))
		}
		deps = append(deps, id)
	}

	for _, id := range deps {
		if _, err := s.store.GetIssue(ctx, id); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, invalid(fmt.Sprintf("dependency %s does not exist", id))
			}
			return nil, err
		}
	}

	if self == "" {
		return deps, nil
	}
	for _, id := range deps {
		reaches, err := s.reaches(ctx, id, self)
		if err != nil {
			return nil, err
		}
		if reaches {
			return nil, invalid(fmt.Sprintf("dependency %s would make a cycle through %s", id, self))
		}
	}
	return deps, nil
}

// reaches reports whether following DependsOn from the issue with ID from
// reaches target. Each issue is visited once. An ID that is not stored is a
// dead end, not an error, because stored issues may name IDs that do not
// exist.
func (s *Service) reaches(ctx context.Context, from, target string) (bool, error) {
	visited := make(map[string]bool)
	stack := []string{from}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[id] {
			continue
		}
		visited[id] = true
		issue, err := s.store.GetIssue(ctx, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return false, err
		}
		for _, dep := range issue.DependsOn {
			if dep == target {
				return true, nil
			}
			stack = append(stack, dep)
		}
	}
	return false, nil
}

func (s *Service) GetIssue(ctx context.Context, id string) (Issue, error) {
	return s.store.GetIssue(ctx, normalizeKey(id))
}

func (s *Service) ListIssues(ctx context.Context, f IssueFilter) ([]Issue, error) {
	f.ProjectKey = normalizeKey(f.ProjectKey)
	var err error
	if f.Page, err = f.Page.resolve(); err != nil {
		return nil, err
	}
	if f.Stage != "" && !f.Stage.Valid() {
		return nil, invalidStage(f.Stage)
	}
	if f.Status != "" && !f.Status.Valid() {
		return nil, invalidStatus(f.Status)
	}
	if !f.Ready {
		return s.store.ListIssues(ctx, f)
	}
	// The page is applied after the readiness filter, so the store must not
	// cut it first. BlockedBy needs every stored issue.
	want := f
	want.Ready = false
	want.Page = Page{}
	matching, err := s.store.ListIssues(ctx, want)
	if err != nil {
		return nil, err
	}
	all, err := s.store.ListIssues(ctx, IssueFilter{})
	if err != nil {
		return nil, err
	}
	ready := []Issue{}
	for _, i := range matching {
		if len(BlockedBy(i, all)) == 0 {
			ready = append(ready, i)
		}
	}
	if f.Offset >= len(ready) {
		return []Issue{}, nil
	}
	return ready[f.Offset:min(f.Offset+f.Limit, len(ready))], nil
}

func (s *Service) UpdateIssue(ctx context.Context, id string, patch IssuePatch) (Issue, error) {
	if patch.Assignee != nil {
		// Empty stays empty here: it unassigns the issue.
		if err := checkOwnName(ctx, strings.TrimSpace(*patch.Assignee)); err != nil {
			return Issue{}, err
		}
	}
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
	if patch.Epic != nil && *patch.Epic < 0 {
		return Issue{}, invalid("epic must be 0 or a positive number")
	}
	if patch.DependsOn != nil {
		deps, err := s.checkDependencies(ctx, normalizeKey(id), *patch.DependsOn)
		if err != nil {
			return Issue{}, err
		}
		patch.DependsOn = &deps
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
		if patch.Epic != nil {
			i.Epic = *patch.Epic
		}
		if patch.DependsOn != nil {
			i.DependsOn = *patch.DependsOn
		}
		i.UpdatedAt = s.now().UTC()
		return nil
	})
}

// ClaimIssue takes a todo issue for assignee and marks it in progress. It
// fails with ErrConflict if the issue is not todo, so of several workers
// racing for one issue exactly one wins.
func (s *Service) ClaimIssue(ctx context.Context, id, assignee string) (Issue, error) {
	assignee, err := ownName(ctx, strings.TrimSpace(assignee))
	if err != nil {
		return Issue{}, err
	}
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

// BlockIssue marks an issue the author cannot finish as blocked. The issue
// keeps its assignee and stage, so it is not claimable and leaves every
// queue. The reason is recorded as a comment whose body starts "BLOCKED: ".
// Author and reason are required. A done issue cannot be blocked.
func (s *Service) BlockIssue(ctx context.Context, id, author, reason string) (Issue, error) {
	author = strings.TrimSpace(author)
	reason = strings.TrimSpace(reason)
	if author == "" {
		return Issue{}, invalid("author is required to block an issue")
	}
	if reason == "" {
		return Issue{}, invalid("reason is required to block an issue")
	}
	issue, err := s.store.UpdateIssue(ctx, normalizeKey(id), func(i *Issue) error {
		if i.Status == StatusDone {
			return fmt.Errorf("issue %s is done and cannot be blocked: %w", i.ID, ErrConflict)
		}
		i.Status = StatusBlocked
		i.UpdatedAt = s.now().UTC()
		return nil
	})
	if err != nil {
		return Issue{}, err
	}
	if _, err := s.store.AddComment(ctx, Comment{
		IssueID:   issue.ID,
		Author:    author,
		Body:      "BLOCKED: " + reason,
		CreatedAt: s.now().UTC(),
	}); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

func (s *Service) AddComment(ctx context.Context, in NewComment) (Comment, error) {
	author, err := ownName(ctx, strings.TrimSpace(in.Author))
	if err != nil {
		return Comment{}, err
	}
	c := Comment{
		IssueID:   normalizeKey(in.IssueID),
		Author:    author,
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

// ListCommentsPage returns one page of an issue's comments, oldest first.
// It returns ErrNotFound if the issue does not exist, and an empty non-nil
// slice past the end.
func (s *Service) ListCommentsPage(ctx context.Context, issueID string, p Page) ([]Comment, error) {
	p, err := p.resolve()
	if err != nil {
		return nil, err
	}
	all, err := s.store.ListComments(ctx, normalizeKey(issueID))
	if err != nil {
		return nil, err
	}
	if p.Offset >= len(all) {
		return []Comment{}, nil
	}
	return all[p.Offset:min(p.Offset+p.Limit, len(all))], nil
}

func (s *Service) EventsSince(ctx context.Context, after int64) ([]Event, error) {
	return s.store.EventsSince(ctx, after)
}

func (s *Service) LastEventID(ctx context.Context) (int64, error) {
	return s.store.LastEventID(ctx)
}

func normalizeKey(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// checkOwnName fails when a principal is present and name names someone
// else. An empty name passes. Without a principal every name passes.
func checkOwnName(ctx context.Context, name string) error {
	p, ok := PrincipalFrom(ctx)
	if !ok || name == "" || name == p.Name {
		return nil
	}
	return invalid(fmt.Sprintf("assignee must be %s or empty", p.Name))
}

// ownName returns the name a write stores. With a principal, an empty name
// becomes the principal's name. Without one, name is returned unchanged.
func ownName(ctx context.Context, name string) (string, error) {
	if err := checkOwnName(ctx, name); err != nil {
		return "", err
	}
	if p, ok := PrincipalFrom(ctx); ok {
		return p.Name, nil
	}
	return name, nil
}

func invalid(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, msg)
}

func invalidStage(s Stage) error {
	return invalid(fmt.Sprintf("stage %q must be one of %s, %s, %s", s, StageRefinement, StageImplementation, StageTesting))
}

func invalidStatus(s Status) error {
	return invalid(fmt.Sprintf("status %q must be one of %s, %s, %s, %s", s, StatusTodo, StatusInProgress, StatusDone, StatusBlocked))
}
