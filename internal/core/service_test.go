package core_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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
	if i.ID != "PLAT-1" || i.Title != "First" || i.Stage != core.StageRefinement || i.Status != core.StatusTodo || !i.CreatedAt.Equal(fixed) {
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
	c, err := svc.AddComment(ctx, core.NewComment{IssueID: "plat-1", Author: " sam ", Body: " looks good "})
	if err != nil {
		t.Fatal(err)
	}
	if c.IssueID != "PLAT-1" || c.Number != 1 || c.Author != "sam" || c.Body != "looks good" || !c.CreatedAt.Equal(fixed) {
		t.Fatalf("comment = %+v", c)
	}
	if _, err := svc.AddComment(ctx, core.NewComment{IssueID: "PLAT-1", Body: " "}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("blank comment = %v; want ErrInvalid", err)
	}
	if _, err := svc.AddComment(ctx, core.NewComment{IssueID: "PLAT-9", Body: "x"}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("comment on missing issue = %v; want ErrNotFound", err)
	}
	if comments, err := svc.ListComments(ctx, "plat-1"); err != nil || len(comments) != 1 {
		t.Errorf("ListComments = %+v, %v", comments, err)
	}

	if _, err := svc.ListIssues(ctx, core.IssueFilter{Status: bad}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("bad filter status = %v; want ErrInvalid", err)
	}
}

func TestStagesAndClaim(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t).Service
	if _, err := svc.CreateProject(ctx, core.NewProject{Key: "PLAT", Name: "Platform"}); err != nil {
		t.Fatal(err)
	}

	i, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "Ready to build", Stage: core.StageImplementation})
	if err != nil || i.Stage != core.StageImplementation {
		t.Fatalf("create with stage = %+v, %v", i, err)
	}
	badStage := core.Stage("review")
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "x", Stage: badStage}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("create with bad stage = %v; want ErrInvalid", err)
	}
	if _, err := svc.UpdateIssue(ctx, i.ID, core.IssuePatch{Stage: &badStage}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("update to bad stage = %v; want ErrInvalid", err)
	}
	if _, err := svc.ListIssues(ctx, core.IssueFilter{Stage: badStage}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("filter on bad stage = %v; want ErrInvalid", err)
	}

	if _, err := svc.ClaimIssue(ctx, i.ID, " "); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("claim without assignee = %v; want ErrInvalid", err)
	}
	if _, err := svc.ClaimIssue(ctx, "PLAT-9", "w"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("claim missing issue = %v; want ErrNotFound", err)
	}

	// Many workers race for one issue; exactly one may win.
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, err := svc.ClaimIssue(ctx, i.ID, "worker")
			switch {
			case err == nil:
				wins.Add(1)
			case !errors.Is(err, core.ErrConflict):
				t.Errorf("losing claim = %v; want ErrConflict", err)
			}
		})
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d workers claimed the issue; want 1", wins.Load())
	}
	claimed, _ := svc.GetIssue(ctx, i.ID)
	if claimed.Status != core.StatusInProgress || claimed.Assignee != "worker" {
		t.Fatalf("claimed issue = %+v", claimed)
	}

	// Handing off to the next loop: next stage, back to todo, unassigned.
	next, todo, nobody := core.StageTesting, core.StatusTodo, ""
	if _, err := svc.UpdateIssue(ctx, i.ID, core.IssuePatch{Stage: &next, Status: &todo, Assignee: &nobody}); err != nil {
		t.Fatal(err)
	}
	queue, err := svc.ListIssues(ctx, core.IssueFilter{Stage: core.StageTesting, Status: core.StatusTodo})
	if err != nil || len(queue) != 1 || queue[0].ID != i.ID {
		t.Fatalf("testing queue = %+v, %v", queue, err)
	}
	if _, err := svc.ClaimIssue(ctx, i.ID, "tester"); err != nil {
		t.Fatalf("claim after handoff: %v", err)
	}
}
