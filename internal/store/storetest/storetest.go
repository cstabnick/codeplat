// Package storetest is the contract every core.Store implementation must pass.
package storetest

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"codeplat/internal/core"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Run exercises a fresh store from newStore in each subtest.
func Run(t *testing.T, newStore func(t *testing.T) core.Store) {
	ctx := context.Background()

	seed := func(t *testing.T) core.Store {
		t.Helper()
		s := newStore(t)
		for _, key := range []string{"AAA", "BBB"} {
			if err := s.CreateProject(ctx, core.Project{Key: key, Name: key + " project", CreatedAt: t0}); err != nil {
				t.Fatalf("CreateProject(%s): %v", key, err)
			}
		}
		return s
	}
	newIssue := func(project, title string) core.Issue {
		return core.Issue{ProjectKey: project, Title: title, Stage: core.StageRefinement, Status: core.StatusTodo, CreatedAt: t0, UpdatedAt: t0}
	}

	t.Run("empty lists are not nil", func(t *testing.T) {
		s := newStore(t)
		projects, err := s.ListProjects(ctx)
		if err != nil || projects == nil || len(projects) != 0 {
			t.Fatalf("ListProjects = %v, %v; want empty non-nil", projects, err)
		}
		issues, err := s.ListIssues(ctx, core.IssueFilter{})
		if err != nil || issues == nil || len(issues) != 0 {
			t.Fatalf("ListIssues = %v, %v; want empty non-nil", issues, err)
		}
	})

	t.Run("projects", func(t *testing.T) {
		s := seed(t)
		err := s.CreateProject(ctx, core.Project{Key: "AAA", Name: "again", CreatedAt: t0})
		if !errors.Is(err, core.ErrConflict) {
			t.Fatalf("duplicate CreateProject = %v; want ErrConflict", err)
		}
		got, err := s.ListProjects(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := core.Project{Key: "AAA", Name: "AAA project", CreatedAt: t0}
		if len(got) != 2 || got[0] != want || got[1].Key != "BBB" {
			t.Fatalf("ListProjects = %+v", got)
		}
	})

	t.Run("issue numbering is per project", func(t *testing.T) {
		s := seed(t)
		var ids []string
		for _, project := range []string{"AAA", "AAA", "BBB"} {
			i, err := s.CreateIssue(ctx, newIssue(project, "x"))
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, i.ID)
		}
		if ids[0] != "AAA-1" || ids[1] != "AAA-2" || ids[2] != "BBB-1" {
			t.Fatalf("ids = %v", ids)
		}
		if _, err := s.CreateIssue(ctx, newIssue("NOPE", "x")); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("CreateIssue in missing project = %v; want ErrNotFound", err)
		}
	})

	t.Run("get round-trips every field", func(t *testing.T) {
		s := seed(t)
		in := newIssue("AAA", "title")
		in.Description, in.Assignee = "desc", "sam"
		created, err := s.CreateIssue(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.GetIssue(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := in
		want.ID, want.Number = "AAA-1", 1
		want.DependsOn = []string{}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(created, want) {
			t.Fatalf("got %+v\ncreated %+v\nwant %+v", got, created, want)
		}
		if _, err := s.GetIssue(ctx, "AAA-99"); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("GetIssue(missing) = %v; want ErrNotFound", err)
		}
	})

	t.Run("list filters and orders", func(t *testing.T) {
		s := seed(t)
		for _, in := range []struct {
			project, assignee string
			status            core.Status
			stage             core.Stage
		}{
			{"BBB", "sam", core.StatusTodo, core.StageTesting},
			{"AAA", "sam", core.StatusDone, core.StageTesting},
			{"AAA", "alex", core.StatusTodo, core.StageImplementation},
		} {
			i := newIssue(in.project, "x")
			i.Assignee, i.Status, i.Stage = in.assignee, in.status, in.stage
			if _, err := s.CreateIssue(ctx, i); err != nil {
				t.Fatal(err)
			}
		}
		for _, tc := range []struct {
			name   string
			filter core.IssueFilter
			want   []string
		}{
			{"all", core.IssueFilter{}, []string{"AAA-1", "AAA-2", "BBB-1"}},
			{"project", core.IssueFilter{ProjectKey: "AAA"}, []string{"AAA-1", "AAA-2"}},
			{"status", core.IssueFilter{Status: core.StatusTodo}, []string{"AAA-2", "BBB-1"}},
			{"stage", core.IssueFilter{Stage: core.StageTesting}, []string{"AAA-1", "BBB-1"}},
			{"stage queue", core.IssueFilter{Stage: core.StageTesting, Status: core.StatusTodo}, []string{"BBB-1"}},
			{"assignee", core.IssueFilter{Assignee: "sam"}, []string{"AAA-1", "BBB-1"}},
			{"combined", core.IssueFilter{ProjectKey: "AAA", Assignee: "sam", Status: core.StatusDone}, []string{"AAA-1"}},
			{"none", core.IssueFilter{Assignee: "nobody"}, []string{}},
		} {
			issues, err := s.ListIssues(ctx, tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for _, i := range issues {
				got = append(got, i.ID)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
			}
			for n := range got {
				if got[n] != tc.want[n] {
					t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
				}
			}
		}
	})

	t.Run("paging", func(t *testing.T) {
		s := seed(t)
		for n := 0; n < 5; n++ {
			if _, err := s.CreateIssue(ctx, newIssue("AAA", "p")); err != nil {
				t.Fatal(err)
			}
		}
		check := func(name string, filter core.IssueFilter, want []string) {
			t.Helper()
			issues, err := s.ListIssues(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			if issues == nil {
				t.Fatalf("%s: got nil, want non-nil", name)
			}
			got := []string{}
			for _, i := range issues {
				got = append(got, i.ID)
			}
			if len(got) != len(want) {
				t.Fatalf("%s: got %v, want %v", name, got, want)
			}
			for n := range got {
				if got[n] != want[n] {
					t.Fatalf("%s: got %v, want %v", name, got, want)
				}
			}
		}
		check("limit and offset", core.IssueFilter{ProjectKey: "AAA", Page: core.Page{Limit: 2, Offset: 2}}, []string{"AAA-3", "AAA-4"})
		check("last page", core.IssueFilter{ProjectKey: "AAA", Page: core.Page{Limit: 2, Offset: 4}}, []string{"AAA-5"})
		check("offset without limit", core.IssueFilter{ProjectKey: "AAA", Page: core.Page{Offset: 4}}, []string{"AAA-5"})
		check("limit without offset", core.IssueFilter{ProjectKey: "AAA", Page: core.Page{Limit: 2}}, []string{"AAA-1", "AAA-2"})
		check("no page", core.IssueFilter{ProjectKey: "AAA"}, []string{"AAA-1", "AAA-2", "AAA-3", "AAA-4", "AAA-5"})
		check("offset past end", core.IssueFilter{ProjectKey: "AAA", Page: core.Page{Limit: 2, Offset: 9}}, []string{})
		check("no done issues", core.IssueFilter{ProjectKey: "AAA", Status: core.StatusDone, Page: core.Page{Limit: 1, Offset: 1}}, []string{})
		if _, err := s.UpdateIssue(ctx, "AAA-2", func(i *core.Issue) error {
			i.Status = core.StatusDone
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		check("window after filter", core.IssueFilter{ProjectKey: "AAA", Status: core.StatusDone, Page: core.Page{Limit: 1, Offset: 0}}, []string{"AAA-2"})
	})

	t.Run("update", func(t *testing.T) {
		s := seed(t)
		created, err := s.CreateIssue(ctx, newIssue("AAA", "before"))
		if err != nil {
			t.Fatal(err)
		}
		later := t0.Add(time.Hour)
		updated, err := s.UpdateIssue(ctx, created.ID, func(i *core.Issue) error {
			i.Title, i.Description, i.Assignee = "after", "d", "sam"
			i.Status, i.UpdatedAt, i.Stage = core.StatusDone, later, core.StageTesting
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetIssue(ctx, created.ID)
		if !reflect.DeepEqual(got, updated) || got.Title != "after" || got.Status != core.StatusDone || got.Stage != core.StageTesting || !got.UpdatedAt.Equal(later) || !got.CreatedAt.Equal(t0) {
			t.Fatalf("after update: %+v", got)
		}

		boom := errors.New("boom")
		_, err = s.UpdateIssue(ctx, created.ID, func(i *core.Issue) error {
			i.Title = "should not persist"
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("UpdateIssue with failing fn = %v; want boom", err)
		}
		if got, _ := s.GetIssue(ctx, created.ID); got.Title != "after" {
			t.Fatalf("failed update persisted: %+v", got)
		}

		_, err = s.UpdateIssue(ctx, "AAA-99", func(*core.Issue) error { return nil })
		if !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("UpdateIssue(missing) = %v; want ErrNotFound", err)
		}
	})

	t.Run("epic and dependencies", func(t *testing.T) {
		s := seed(t)
		plain, err := s.CreateIssue(ctx, newIssue("AAA", "plain"))
		if err != nil {
			t.Fatal(err)
		}
		if plain.Epic != 0 || plain.DependsOn == nil || len(plain.DependsOn) != 0 {
			t.Fatalf("CreateIssue without epic = %+v; want epic 0 and empty non-nil DependsOn", plain)
		}
		in := newIssue("AAA", "epic")
		in.Epic, in.DependsOn = 2, []string{"AAA-1"}
		created, err := s.CreateIssue(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.GetIssue(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Epic != 2 || !reflect.DeepEqual(got.DependsOn, []string{"AAA-1"}) {
			t.Fatalf("GetIssue = epic %d, depends_on %v; want epic 2, depends_on [AAA-1]", got.Epic, got.DependsOn)
		}
		plainGot, err := s.GetIssue(ctx, plain.ID)
		if err != nil {
			t.Fatal(err)
		}
		if plainGot.Epic != 0 || plainGot.DependsOn == nil || len(plainGot.DependsOn) != 0 {
			t.Fatalf("GetIssue without epic = %+v; want epic 0 and empty non-nil DependsOn", plainGot)
		}

		updated, err := s.UpdateIssue(ctx, created.ID, func(i *core.Issue) error {
			i.Epic, i.DependsOn = 3, []string{"BBB-1", "AAA-1"}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if updated.Epic != 3 || !reflect.DeepEqual(updated.DependsOn, []string{"BBB-1", "AAA-1"}) {
			t.Fatalf("UpdateIssue returned epic %d, depends_on %v", updated.Epic, updated.DependsOn)
		}
		got, err = s.GetIssue(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, updated) {
			t.Fatalf("after update: got %+v, want %+v", got, updated)
		}
		listed, err := s.ListIssues(ctx, core.IssueFilter{ProjectKey: "AAA"})
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 2 || !reflect.DeepEqual(listed[1], updated) {
			t.Fatalf("ListIssues after update = %+v; want second issue %+v", listed, updated)
		}

		cleared, err := s.UpdateIssue(ctx, created.ID, func(i *core.Issue) error {
			i.DependsOn = []string{}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err = s.GetIssue(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.DependsOn == nil || len(got.DependsOn) != 0 || cleared.DependsOn == nil {
			t.Fatalf("after clearing, DependsOn = %v; want empty non-nil", got.DependsOn)
		}

		issues, err := s.ListIssues(ctx, core.IssueFilter{ProjectKey: "AAA"})
		if err != nil {
			t.Fatal(err)
		}
		if len(issues) != 2 {
			t.Fatalf("ListIssues = %+v; want 2 issues", issues)
		}
		if issues[0].Epic != 0 || issues[0].DependsOn == nil || len(issues[0].DependsOn) != 0 {
			t.Fatalf("listed plain issue = %+v", issues[0])
		}
		if issues[1].Epic != 3 || issues[1].DependsOn == nil || len(issues[1].DependsOn) != 0 {
			t.Fatalf("listed epic issue = %+v; want epic 3 and no dependencies after clear", issues[1])
		}
	})

	t.Run("blocked status", func(t *testing.T) {
		s := seed(t)
		blocked, err := s.CreateIssue(ctx, newIssue("AAA", "blocked"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateIssue(ctx, newIssue("AAA", "todo")); err != nil {
			t.Fatal(err)
		}
		later := t0.Add(time.Hour)
		if _, err := s.UpdateIssue(ctx, blocked.ID, func(i *core.Issue) error {
			i.Status, i.Assignee, i.Stage, i.UpdatedAt = core.StatusBlocked, "sam", core.StageTesting, later
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetIssue(ctx, blocked.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != core.StatusBlocked || got.Assignee != "sam" || got.Stage != core.StageTesting {
			t.Fatalf("GetIssue after block = %+v; want status blocked, assignee sam, stage testing", got)
		}
		issues, err := s.ListIssues(ctx, core.IssueFilter{Status: core.StatusBlocked})
		if err != nil {
			t.Fatal(err)
		}
		if len(issues) != 1 || issues[0].ID != blocked.ID {
			t.Fatalf("ListIssues(blocked) = %+v; want only %s", issues, blocked.ID)
		}
	})

	t.Run("comments", func(t *testing.T) {
		s := seed(t)
		for _, project := range []string{"AAA", "AAA"} {
			if _, err := s.CreateIssue(ctx, newIssue(project, "x")); err != nil {
				t.Fatal(err)
			}
		}
		empty, err := s.ListComments(ctx, "AAA-1")
		if err != nil || empty == nil || len(empty) != 0 {
			t.Fatalf("ListComments on new issue = %v, %v; want empty non-nil", empty, err)
		}

		first, err := s.AddComment(ctx, core.Comment{IssueID: "AAA-1", Author: "sam", Body: "one", CreatedAt: t0})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.AddComment(ctx, core.Comment{IssueID: "AAA-1", Body: "two", CreatedAt: t0.Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
		other, err := s.AddComment(ctx, core.Comment{IssueID: "AAA-2", Body: "elsewhere", CreatedAt: t0})
		if err != nil {
			t.Fatal(err)
		}
		if other.Number != 1 {
			t.Fatalf("numbering is not per issue: %+v", other)
		}

		got, err := s.ListComments(ctx, "AAA-1")
		if err != nil {
			t.Fatal(err)
		}
		want := core.Comment{IssueID: "AAA-1", Number: 1, Author: "sam", Body: "one", CreatedAt: t0}
		if len(got) != 2 || got[0] != want || first != want || got[1].Number != 2 || got[1].Body != "two" {
			t.Fatalf("ListComments = %+v", got)
		}

		if _, err := s.AddComment(ctx, core.Comment{IssueID: "AAA-99", Body: "x", CreatedAt: t0}); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("AddComment(missing issue) = %v; want ErrNotFound", err)
		}
		if _, err := s.ListComments(ctx, "AAA-99"); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("ListComments(missing issue) = %v; want ErrNotFound", err)
		}
	})

	t.Run("concurrent comments get distinct numbers", func(t *testing.T) {
		s := seed(t)
		if _, err := s.CreateIssue(ctx, newIssue("AAA", "x")); err != nil {
			t.Fatal(err)
		}
		const n = 20
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for range n {
			wg.Go(func() {
				_, err := s.AddComment(ctx, core.Comment{IssueID: "AAA-1", Body: "x", CreatedAt: t0})
				errs <- err
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		comments, err := s.ListComments(ctx, "AAA-1")
		if err != nil || len(comments) != n {
			t.Fatalf("got %d comments, %v; want %d", len(comments), err, n)
		}
		for idx, c := range comments {
			if c.Number != idx+1 {
				t.Fatalf("comment %d has number %d", idx, c.Number)
			}
		}
	})

	t.Run("concurrent creates get distinct numbers", func(t *testing.T) {
		s := seed(t)
		const n = 20
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for range n {
			wg.Go(func() {
				_, err := s.CreateIssue(ctx, newIssue("AAA", "x"))
				errs <- err
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		issues, err := s.ListIssues(ctx, core.IssueFilter{ProjectKey: "AAA"})
		if err != nil {
			t.Fatal(err)
		}
		if len(issues) != n {
			t.Fatalf("got %d issues, want %d", len(issues), n)
		}
		for idx, i := range issues {
			if i.Number != idx+1 {
				t.Fatalf("issue %d has number %d", idx, i.Number)
			}
		}
	})

	t.Run("events", func(t *testing.T) {
		s := newStore(t)
		if last, err := s.LastEventID(ctx); err != nil || last != 0 {
			t.Fatalf("new store LastEventID = %d, %v; want 0", last, err)
		}
		if evs, err := s.EventsSince(ctx, 0); err != nil || evs == nil || len(evs) != 0 {
			t.Fatalf("new store EventsSince = %v, %v; want empty non-nil", evs, err)
		}
		if err := s.CreateProject(ctx, core.Project{Key: "AAA", Name: "AAA project", CreatedAt: t0}); err != nil {
			t.Fatal(err)
		}
		issue, err := s.CreateIssue(ctx, newIssue("AAA", "x"))
		if err != nil {
			t.Fatal(err)
		}
		later := t0.Add(time.Hour)
		if _, err := s.UpdateIssue(ctx, issue.ID, func(i *core.Issue) error {
			i.Title, i.UpdatedAt = "y", later
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		commentAt := t0.Add(2 * time.Hour)
		if _, err := s.AddComment(ctx, core.Comment{IssueID: issue.ID, Body: "hi", CreatedAt: commentAt}); err != nil {
			t.Fatal(err)
		}

		want := []core.Event{
			{ID: 1, Type: core.EventProjectCreated, ProjectKey: "AAA", At: t0},
			{ID: 2, Type: core.EventIssueCreated, ProjectKey: "AAA", IssueID: "AAA-1", At: t0},
			{ID: 3, Type: core.EventIssueUpdated, ProjectKey: "AAA", IssueID: "AAA-1", At: later},
			{ID: 4, Type: core.EventCommentAdded, ProjectKey: "AAA", IssueID: "AAA-1", At: commentAt},
		}
		got, err := s.EventsSince(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("EventsSince(0) = %+v; want %d events", got, len(want))
		}
		for n := range want {
			g, w := got[n], want[n]
			if g.ID != w.ID || g.Type != w.Type || g.ProjectKey != w.ProjectKey || g.IssueID != w.IssueID || !g.At.Equal(w.At) {
				t.Fatalf("event %d = %+v; want %+v", n, g, w)
			}
		}
		if last, err := s.LastEventID(ctx); err != nil || last != 4 {
			t.Fatalf("LastEventID = %d, %v; want 4", last, err)
		}

		tail, err := s.EventsSince(ctx, 2)
		if err != nil || len(tail) != 2 || tail[0].ID != 3 || tail[1].ID != 4 {
			t.Fatalf("EventsSince(2) = %+v, %v; want IDs 3 and 4", tail, err)
		}

		if err := s.CreateProject(ctx, core.Project{Key: "AAA", Name: "again", CreatedAt: t0}); !errors.Is(err, core.ErrConflict) {
			t.Fatalf("duplicate CreateProject = %v; want ErrConflict", err)
		}
		if _, err := s.CreateIssue(ctx, newIssue("NOPE", "x")); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("CreateIssue in missing project = %v; want ErrNotFound", err)
		}
		boom := errors.New("boom")
		if _, err := s.UpdateIssue(ctx, issue.ID, func(*core.Issue) error { return boom }); !errors.Is(err, boom) {
			t.Fatalf("UpdateIssue with failing fn = %v; want boom", err)
		}
		if _, err := s.AddComment(ctx, core.Comment{IssueID: "AAA-99", Body: "x", CreatedAt: t0}); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("AddComment(missing issue) = %v; want ErrNotFound", err)
		}
		if last, err := s.LastEventID(ctx); err != nil || last != 4 {
			t.Fatalf("failed calls recorded events: LastEventID = %d, %v; want 4", last, err)
		}
	})
}
