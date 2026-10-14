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

func TestServiceEvents(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t).Service

	if _, err := svc.CreateProject(ctx, core.NewProject{Key: "EVT", Name: "Events"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "EVT", Title: "x"}); err != nil {
		t.Fatal(err)
	}
	last, err := svc.LastEventID(ctx)
	if err != nil || last != 2 {
		t.Fatalf("LastEventID = %d, %v; want 2", last, err)
	}
	evs, err := svc.EventsSince(ctx, 1)
	if err != nil || len(evs) != 1 || evs[0].Type != core.EventIssueCreated || evs[0].IssueID != "EVT-1" {
		t.Fatalf("EventsSince(1) = %+v, %v; want one issue.created for EVT-1", evs, err)
	}
}

func TestBlockIssue(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t).Service

	if _, err := svc.CreateProject(ctx, core.NewProject{Key: "PLAT", Name: "Platform"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "x", Assignee: "sam", Stage: core.StageTesting}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.BlockIssue(ctx, "plat-1", "sam", " needs a spec ")
	if err != nil {
		t.Fatalf("BlockIssue: %v", err)
	}
	if got.Status != core.StatusBlocked || got.Assignee != "sam" || got.Stage != core.StageTesting {
		t.Fatalf("BlockIssue = %+v; want status blocked, assignee sam, stage testing", got)
	}
	comments, err := svc.ListComments(ctx, "PLAT-1")
	if err != nil || len(comments) != 1 || comments[0].Author != "sam" || comments[0].Body != "BLOCKED: needs a spec" {
		t.Fatalf("ListComments = %+v, %v; want one BLOCKED comment by sam", comments, err)
	}

	for _, tc := range []struct{ author, reason string }{
		{"", "reason"},
		{"sam", ""},
		{"sam", "   "},
	} {
		if _, err := svc.BlockIssue(ctx, "PLAT-1", tc.author, tc.reason); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("BlockIssue(author=%q, reason=%q) = %v; want ErrInvalid", tc.author, tc.reason, err)
		}
	}
	if comments, _ := svc.ListComments(ctx, "PLAT-1"); len(comments) != 1 {
		t.Errorf("invalid BlockIssue added comments: %+v", comments)
	}

	if _, err := svc.ClaimIssue(ctx, "PLAT-1", "tester"); !errors.Is(err, core.ErrConflict) {
		t.Errorf("claim blocked issue = %v; want ErrConflict", err)
	}

	done := core.StatusDone
	if _, err := svc.UpdateIssue(ctx, "PLAT-1", core.IssuePatch{Status: &done}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BlockIssue(ctx, "PLAT-1", "sam", "too late"); !errors.Is(err, core.ErrConflict) {
		t.Errorf("block done issue = %v; want ErrConflict", err)
	}
	if comments, _ := svc.ListComments(ctx, "PLAT-1"); len(comments) != 1 {
		t.Errorf("blocking a done issue added comments: %+v", comments)
	}

	if _, err := svc.BlockIssue(ctx, "PLAT-99", "sam", "x"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("block missing issue = %v; want ErrNotFound", err)
	}
}

func TestPaging(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t).Service
	if _, err := svc.CreateProject(ctx, core.NewProject{Key: "PAGE", Name: "Paging"}); err != nil {
		t.Fatal(err)
	}
	issue, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PAGE", Title: "Paged"})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"comment 1", "comment 2", "comment 3", "comment 4", "comment 5"} {
		if _, err := svc.AddComment(ctx, core.NewComment{IssueID: issue.ID, Author: "w", Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	numbers := func(cs []core.Comment) []int {
		var ns []int
		for _, c := range cs {
			ns = append(ns, c.Number)
		}
		return ns
	}
	same := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	for _, tc := range []struct {
		name string
		page core.Page
		want []int
	}{
		{"first two", core.Page{Limit: 2}, []int{1, 2}},
		{"offset four", core.Page{Limit: 2, Offset: 4}, []int{5}},
		{"default limit", core.Page{}, []int{1, 2, 3, 4, 5}},
		{"past the end", core.Page{Offset: 5}, nil},
	} {
		got, err := svc.ListCommentsPage(ctx, issue.ID, tc.page)
		if err != nil || got == nil && tc.want != nil || !same(numbers(got), tc.want) {
			t.Errorf("%s: ListCommentsPage(%+v) = %v, %v; want numbers %v", tc.name, tc.page, numbers(got), err, tc.want)
		}
	}
	past, err := svc.ListCommentsPage(ctx, issue.ID, core.Page{Offset: 5})
	if err != nil || past == nil {
		t.Errorf("past the end = %#v, %v; want an empty non-nil slice", past, err)
	}

	for _, p := range []core.Page{{Limit: 201}, {Limit: -1}, {Offset: -1}} {
		if _, err := svc.ListCommentsPage(ctx, issue.ID, p); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("ListCommentsPage(%+v) = %v; want ErrInvalid", p, err)
		}
	}
	if _, err := svc.ListIssues(ctx, core.IssueFilter{Page: core.Page{Limit: 201}}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("ListIssues with limit 201 = %v; want ErrInvalid", err)
	}
	if _, err := svc.ListCommentsPage(ctx, "NOPE-1", core.Page{}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("ListCommentsPage on missing issue = %v; want ErrNotFound", err)
	}
}

func TestPrincipalRules(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t).Service
	if _, err := svc.CreateProject(ctx, core.NewProject{Key: "PLAT", Name: "Platform"}); err != nil {
		t.Fatal(err)
	}
	as := core.WithPrincipal(ctx, core.Principal{Name: "alice"})

	c, err := svc.AddComment(as, core.NewComment{IssueID: "PLAT-1", Body: "no issue yet"})
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("comment on missing issue = %v, %+v; want ErrNotFound", err, c)
	}
	i, err := svc.CreateIssue(as, core.NewIssue{ProjectKey: "PLAT", Title: "Owned", Stage: core.StageImplementation})
	if err != nil || i.Assignee != "alice" {
		t.Fatalf("create with empty assignee = %+v, %v; want assignee alice", i, err)
	}
	c, err = svc.AddComment(as, core.NewComment{IssueID: i.ID, Body: "hello"})
	if err != nil || c.Author != "alice" {
		t.Fatalf("comment with empty author = %+v, %v; want author alice", c, err)
	}
	if _, err := svc.AddComment(as, core.NewComment{IssueID: i.ID, Author: "bob", Body: "x"}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("comment as bob = %v; want ErrInvalid", err)
	}
	if _, err := svc.CreateIssue(as, core.NewIssue{ProjectKey: "PLAT", Title: "x", Assignee: "bob"}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("create assigned to bob = %v; want ErrInvalid", err)
	}

	todo, err := svc.CreateIssue(as, core.NewIssue{ProjectKey: "PLAT", Title: "Claimable", Stage: core.StageImplementation})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ClaimIssue(as, todo.ID, "bob"); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("claim as bob = %v; want ErrInvalid", err)
	}
	if still, _ := svc.GetIssue(ctx, todo.ID); still.Status != core.StatusTodo {
		t.Errorf("failed claim changed status to %s", still.Status)
	}
	// An empty assignee claims as the principal.
	claimed, err := svc.ClaimIssue(as, todo.ID, "")
	if err != nil || claimed.Status != core.StatusInProgress || claimed.Assignee != "alice" {
		t.Fatalf("claim with empty assignee = %+v, %v; want in_progress for alice", claimed, err)
	}

	bob := "bob"
	if _, err := svc.UpdateIssue(as, todo.ID, core.IssuePatch{Assignee: &bob}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("update assignee to bob = %v; want ErrInvalid", err)
	}
	empty := ""
	got, err := svc.UpdateIssue(as, todo.ID, core.IssuePatch{Assignee: &empty})
	if err != nil || got.Assignee != "" {
		t.Errorf("unassign = %+v, %v; want empty assignee", got, err)
	}

	// Without a principal the names are not restricted.
	if _, err := svc.AddComment(ctx, core.NewComment{IssueID: todo.ID, Author: "bob", Body: "fine"}); err != nil {
		t.Errorf("comment as bob without principal = %v", err)
	}
	if _, err := svc.UpdateIssue(ctx, todo.ID, core.IssuePatch{Assignee: &bob}); err != nil {
		t.Errorf("assign bob without principal = %v", err)
	}
}

func TestEpicsAndDependencies(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t).Service
	for _, key := range []string{"PLAT", "CYC"} {
		if _, err := svc.CreateProject(ctx, core.NewProject{Key: key, Name: key}); err != nil {
			t.Fatal(err)
		}
	}
	same := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	ids := func(is []core.Issue) []string {
		out := []string{}
		for _, i := range is {
			out = append(out, i.ID)
		}
		return out
	}

	// Bad input on create.
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "x", Epic: -1}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("create with negative epic = %v; want ErrInvalid", err)
	}
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "x", DependsOn: []string{"PLAT-99"}}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("create with missing dependency = %v; want ErrInvalid", err)
	}
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "x", DependsOn: []string{" "}}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("create with blank dependency = %v; want ErrInvalid", err)
	}

	a, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "B", DependsOn: []string{" plat-1 ", "PLAT-1"}})
	if err != nil || !same(b.DependsOn, []string{"PLAT-1"}) {
		t.Fatalf("create B = %+v, %v; want one dependency on PLAT-1", b, err)
	}
	c, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "C", Epic: 1})
	if err != nil || c.Epic != 1 {
		t.Fatalf("create C = %+v, %v; want epic 1", c, err)
	}
	d, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "D", Epic: 2})
	if err != nil || d.Epic != 2 {
		t.Fatalf("create D = %+v, %v; want epic 2", d, err)
	}

	// Bad input on update.
	neg := -1
	if _, err := svc.UpdateIssue(ctx, a.ID, core.IssuePatch{Epic: &neg}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("update to negative epic = %v; want ErrInvalid", err)
	}
	self := []string{a.ID}
	if _, err := svc.UpdateIssue(ctx, a.ID, core.IssuePatch{DependsOn: &self}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("self-dependency = %v; want ErrInvalid", err)
	}
	missing := []string{"PLAT-99"}
	if _, err := svc.UpdateIssue(ctx, a.ID, core.IssuePatch{DependsOn: &missing}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("update to missing dependency = %v; want ErrInvalid", err)
	}
	lower := []string{"plat-3", "PLAT-3"}
	got, err := svc.UpdateIssue(ctx, d.ID, core.IssuePatch{DependsOn: &lower})
	if err != nil || !same(got.DependsOn, []string{"PLAT-3"}) {
		t.Fatalf("update D = %+v, %v; want one dependency on PLAT-3", got, err)
	}

	// Cycles: X depends on Y; then Y may not depend on X. A longer loop
	// X -> Z -> Y -> X is also refused.
	x, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "CYC", Title: "X"})
	if err != nil {
		t.Fatal(err)
	}
	y, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "CYC", Title: "Y"})
	if err != nil {
		t.Fatal(err)
	}
	z, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "CYC", Title: "Z"})
	if err != nil {
		t.Fatal(err)
	}
	xDeps := []string{y.ID}
	if _, err := svc.UpdateIssue(ctx, x.ID, core.IssuePatch{DependsOn: &xDeps}); err != nil {
		t.Fatalf("X depends on Y: %v", err)
	}
	yDeps := []string{x.ID}
	if _, err := svc.UpdateIssue(ctx, y.ID, core.IssuePatch{DependsOn: &yDeps}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("two-step cycle = %v; want ErrInvalid", err)
	}
	if now, _ := svc.GetIssue(ctx, y.ID); len(now.DependsOn) != 0 {
		t.Errorf("rejected update changed Y to %v", now.DependsOn)
	}
	zDeps := []string{x.ID}
	if _, err := svc.UpdateIssue(ctx, z.ID, core.IssuePatch{DependsOn: &zDeps}); err != nil {
		t.Fatalf("Z depends on X: %v", err)
	}
	loop := []string{z.ID}
	if _, err := svc.UpdateIssue(ctx, y.ID, core.IssuePatch{DependsOn: &loop}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("three-step cycle = %v; want ErrInvalid", err)
	}

	// Ready. PLAT state: A (no deps), B (needs A), C (epic 1), D (epic 2,
	// needs C through its epic and its dependency).
	ready := func(p core.Page) []core.Issue {
		t.Helper()
		list, err := svc.ListIssues(ctx, core.IssueFilter{ProjectKey: "PLAT", Stage: core.StageRefinement, Status: core.StatusTodo, Ready: true, Page: p})
		if err != nil {
			t.Fatalf("ListIssues ready %+v: %v", p, err)
		}
		return list
	}
	all, err := svc.ListIssues(ctx, core.IssueFilter{ProjectKey: "PLAT", Stage: core.StageRefinement, Status: core.StatusTodo})
	if err != nil || !same(ids(all), []string{"PLAT-1", "PLAT-2", "PLAT-3", "PLAT-4"}) {
		t.Errorf("unfiltered list = %v, %v; want all four PLAT issues", ids(all), err)
	}
	if list := ready(core.Page{}); !same(ids(list), []string{"PLAT-1", "PLAT-3"}) {
		t.Errorf("ready = %v; want [PLAT-1 PLAT-3]", ids(list))
	}
	// PLAT-2 sits between the two ready issues. A store-side page would
	// return it and leave the page short, so the offset must skip it.
	if list := ready(core.Page{Limit: 1}); !same(ids(list), []string{"PLAT-1"}) {
		t.Errorf("ready limit 1 = %v; want [PLAT-1]", ids(list))
	}
	if list := ready(core.Page{Limit: 1, Offset: 1}); !same(ids(list), []string{"PLAT-3"}) {
		t.Errorf("ready limit 1 offset 1 = %v; want [PLAT-3]", ids(list))
	}
	if list := ready(core.Page{Offset: 2}); list == nil || len(list) != 0 {
		t.Errorf("ready past the end = %#v; want an empty non-nil slice", list)
	}

	blockedToo, err := svc.ListIssues(ctx, core.IssueFilter{ProjectKey: "PLAT", Stage: core.StageRefinement, Status: core.StatusTodo})
	if err != nil || len(blockedToo) != 4 {
		t.Errorf("Ready false = %v, %v; want all four", ids(blockedToo), err)
	}

	// Finishing C unblocks D. Finishing A unblocks B.
	done := core.StatusDone
	if _, err := svc.UpdateIssue(ctx, c.ID, core.IssuePatch{Status: &done}); err != nil {
		t.Fatal(err)
	}
	if list := ready(core.Page{}); !same(ids(list), []string{"PLAT-1", "PLAT-4"}) {
		t.Errorf("ready after C is done = %v; want [PLAT-1 PLAT-4]", ids(list))
	}
	if _, err := svc.UpdateIssue(ctx, a.ID, core.IssuePatch{Status: &done}); err != nil {
		t.Fatal(err)
	}
	if list := ready(core.Page{}); !same(ids(list), []string{"PLAT-2", "PLAT-4"}) {
		t.Errorf("ready after A is done = %v; want [PLAT-2 PLAT-4]", ids(list))
	}
}

func TestParentsAndEpicNames(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t).Service
	for _, key := range []string{"PLAT", "OTHER"} {
		if _, err := svc.CreateProject(ctx, core.NewProject{Key: key, Name: key}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "OTHER", Title: "elsewhere"}); err != nil {
		t.Fatal(err)
	}

	// A goal (PLAT-1) with one part (PLAT-2), and a plain issue (PLAT-3).
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "goal"}); err != nil {
		t.Fatal(err)
	}
	part, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "part", Parent: " plat-1 "})
	if err != nil || part.Parent != "PLAT-1" {
		t.Fatalf("create PLAT-2 = %+v, %v; want parent PLAT-1", part, err)
	}
	if got, err := svc.GetIssue(ctx, "PLAT-2"); err != nil || got.Parent != "PLAT-1" {
		t.Fatalf("read PLAT-2 = %+v, %v; want parent PLAT-1", got, err)
	}
	if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "plain"}); err != nil {
		t.Fatal(err)
	}

	// Refused parents on create.
	for _, tc := range []struct{ name, parent string }{
		{"missing parent", "PLAT-99"},
		{"parent in another project", "OTHER-1"},
		{"parent is a part", "PLAT-2"},
	} {
		if _, err := svc.CreateIssue(ctx, core.NewIssue{ProjectKey: "PLAT", Title: "x", Parent: tc.parent}); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("create with %s (%s) = %v; want ErrInvalid", tc.name, tc.parent, err)
		}
	}

	// Refused parents on update.
	self, plain, empty := "PLAT-1", "PLAT-3", ""
	if _, err := svc.UpdateIssue(ctx, "PLAT-1", core.IssuePatch{Parent: &self}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("update PLAT-1 to be its own parent = %v; want ErrInvalid", err)
	}
	if _, err := svc.UpdateIssue(ctx, "PLAT-1", core.IssuePatch{Parent: &plain}); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("give PLAT-1 (has parts) a parent = %v; want ErrInvalid", err)
	}

	// Clearing a parent, and a patch without Parent leaves it alone.
	title := "part renamed"
	if _, err := svc.UpdateIssue(ctx, "PLAT-2", core.IssuePatch{Title: &title}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetIssue(ctx, "PLAT-2"); got.Parent != "PLAT-1" || got.Title != title {
		t.Fatalf("patch without Parent = %+v; want parent PLAT-1 kept", got)
	}
	if _, err := svc.UpdateIssue(ctx, "PLAT-2", core.IssuePatch{Parent: &empty}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetIssue(ctx, "PLAT-2"); got.Parent != "" {
		t.Errorf("cleared parent = %q; want empty", got.Parent)
	}

	// PLAT-1 has no parts once PLAT-2 is cleared, so it may now take a parent.
	if _, err := svc.UpdateIssue(ctx, "PLAT-1", core.IssuePatch{Parent: &plain}); err != nil {
		t.Errorf("give PLAT-1 (no parts) parent PLAT-3 = %v; want success", err)
	}
	if _, err := svc.UpdateIssue(ctx, "PLAT-1", core.IssuePatch{Parent: &empty}); err != nil {
		t.Fatal(err)
	}

	// Epic names.
	for _, e := range []core.Epic{
		{ProjectKey: "PLAT", Number: 0, Name: "Core"},
		{ProjectKey: "PLAT", Number: 1, Name: "   "},
		{ProjectKey: "PLAT", Number: 1, Name: "Core", Parent: "PLAT-99"},
		{ProjectKey: "", Number: 1, Name: "Core"},
	} {
		if _, err := svc.SetEpic(ctx, e); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("SetEpic(%+v) = %v; want ErrInvalid", e, err)
		}
	}
	if _, err := svc.SetEpic(ctx, core.Epic{ProjectKey: "NOPE", Number: 1, Name: "Core"}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("SetEpic in missing project = %v; want ErrNotFound", err)
	}
	got, err := svc.SetEpic(ctx, core.Epic{ProjectKey: "plat", Parent: "plat-1", Number: 1, Name: " Core "})
	if err != nil || got.ProjectKey != "PLAT" || got.Parent != "PLAT-1" || got.Name != "Core" {
		t.Fatalf("SetEpic = %+v, %v; want PLAT, PLAT-1, Core", got, err)
	}
	epics, err := svc.ListEpics(ctx, "plat")
	if err != nil || len(epics) != 1 || epics[0].Name != "Core" || epics[0].Parent != "PLAT-1" {
		t.Errorf("ListEpics(plat) = %+v, %v; want the Core epic", epics, err)
	}
}
