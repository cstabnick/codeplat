// Package memory is a core.Store that keeps everything in process memory.
package memory

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"codeplat/internal/core"
)

type Store struct {
	mu         sync.Mutex
	projects   map[string]core.Project
	nextNumber map[string]int
	issues     map[string]core.Issue
	comments   map[string][]core.Comment
}

func New() *Store {
	return &Store{
		projects:   map[string]core.Project{},
		nextNumber: map[string]int{},
		issues:     map[string]core.Issue{},
		comments:   map[string][]core.Comment{},
	}
}

func (s *Store) CreateProject(_ context.Context, p core.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[p.Key]; ok {
		return fmt.Errorf("project %s already exists: %w", p.Key, core.ErrConflict)
	}
	s.projects[p.Key] = p
	s.nextNumber[p.Key] = 1
	return nil
}

func (s *Store) ListProjects(_ context.Context) ([]core.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]core.Project, 0, len(s.projects))
	for _, p := range s.projects {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b core.Project) int { return cmp.Compare(a.Key, b.Key) })
	return out, nil
}

func (s *Store) CreateIssue(_ context.Context, i core.Issue) (core.Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[i.ProjectKey]; !ok {
		return core.Issue{}, fmt.Errorf("project %s: %w", i.ProjectKey, core.ErrNotFound)
	}
	i.Number = s.nextNumber[i.ProjectKey]
	s.nextNumber[i.ProjectKey]++
	i.ID = core.IssueID(i.ProjectKey, i.Number)
	s.issues[i.ID] = i
	return i, nil
}

func (s *Store) GetIssue(_ context.Context, id string) (core.Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.issues[id]
	if !ok {
		return core.Issue{}, fmt.Errorf("issue %s: %w", id, core.ErrNotFound)
	}
	return i, nil
}

func (s *Store) ListIssues(_ context.Context, f core.IssueFilter) ([]core.Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []core.Issue{}
	for _, i := range s.issues {
		if f.ProjectKey != "" && i.ProjectKey != f.ProjectKey {
			continue
		}
		if f.Stage != "" && i.Stage != f.Stage {
			continue
		}
		if f.Status != "" && i.Status != f.Status {
			continue
		}
		if f.Assignee != "" && i.Assignee != f.Assignee {
			continue
		}
		out = append(out, i)
	}
	slices.SortFunc(out, func(a, b core.Issue) int {
		return cmp.Or(cmp.Compare(a.ProjectKey, b.ProjectKey), cmp.Compare(a.Number, b.Number))
	})
	return out, nil
}

func (s *Store) UpdateIssue(_ context.Context, id string, fn func(*core.Issue) error) (core.Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.issues[id]
	if !ok {
		return core.Issue{}, fmt.Errorf("issue %s: %w", id, core.ErrNotFound)
	}
	if err := fn(&i); err != nil {
		return core.Issue{}, err
	}
	s.issues[id] = i
	return i, nil
}

func (s *Store) AddComment(_ context.Context, c core.Comment) (core.Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.issues[c.IssueID]; !ok {
		return core.Comment{}, fmt.Errorf("issue %s: %w", c.IssueID, core.ErrNotFound)
	}
	c.Number = len(s.comments[c.IssueID]) + 1
	s.comments[c.IssueID] = append(s.comments[c.IssueID], c)
	return c, nil
}

func (s *Store) ListComments(_ context.Context, issueID string) ([]core.Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.issues[issueID]; !ok {
		return nil, fmt.Errorf("issue %s: %w", issueID, core.ErrNotFound)
	}
	return append([]core.Comment{}, s.comments[issueID]...), nil
}
