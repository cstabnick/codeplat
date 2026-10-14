package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeplat/internal/apptest"
	"codeplat/internal/core"
)

func TestServiceRules(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t).Service
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	p, err := svc.CreateProject(ctx, core.NewProject{Key: " plat ", Name: " Platform "})
	if err != nil {
		t.Fatal(err)
	}
	if p.Key != "PLAT" || p.Name != "Platform" || !p.CreatedAt.Equal(fixed) {
		t.Fatalf("project = %+v", p)
	}

	for _, key := range []string{"", "A", "1AB", "A-B", "ABCDEFGHIJK"} {
		if _, err := svc.CreateProject(ctx, core.NewProject{Key: key, Name: "x"}); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("CreateProject(key=%q) = %v; want ErrInvalid", key, err)
		}
	}
	if _, err := svc.CreateProject(ctx, core.NewProject{Key: "OK", Name: " "}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("blank name = %v; want ErrInvalid", err)
	}
	if _, err := svc.CreateProject(ctx, core.NewProject{Key: "plat", Name: "dup"}); !errors.Is(err, core.ErrConflict) {
		t.Errorf("duplicate = %v; want ErrConflict", err)
	}

	i, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "plat", Title: " First ", Assignee: "sam"})
	if err != nil {
		t.Fatal(err)
	}
	if i.ID != "PLAT-1" || i.Title != "First" || i.Status != core.StatusTodo || !i.CreatedAt.Equal(fixed) {
		t.Fatalf("issue = %+v", i)
	}
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: " "}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("blank title = %v; want ErrInvalid", err)
	}
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "NOPE", Title: "x"}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("missing project = %v; want ErrNotFound", err)
	}

	if got, err := svc.GetIssue(ctx, "plat-1"); err != nil || got.ID != "PLAT-1" {
		t.Errorf("GetIssue(lowercase) = %+v, %v", got, err)
	}

	done, empty, bad := core.StatusDone, "", core.Status("closed")
	got, err := svc.UpdateIssue(ctx, "PLAT-1", core.IssuePatch{Status: &done, Assignee: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != core.StatusDone || got.Assignee != "" || got.Title != "First" {
		t.Fatalf("after patch = %+v", got)
	}
	if _, err := svc.UpdateIssue(ctx, "PLAT-1", core.IssuePatch{Status: &bad}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("bad status = %v; want ErrInvalid", err)
	}
	if _, err := svc.UpdateIssue(ctx, "PLAT-1", core.IssuePatch{Title: &empty}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("empty title = %v; want ErrInvalid", err)
	}
	if _, err := svc.ListIssues(ctx, core.IssueFilter{Status: bad}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("bad filter status = %v; want ErrInvalid", err)
	}
}
