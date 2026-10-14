package core

import (
	"reflect"
	"testing"
)

func TestBlockedBy(t *testing.T) {
	issue := func(id, project string, epic int, status Status, deps ...string) Issue {
		return Issue{ID: id, ProjectKey: project, Epic: epic, Status: status, DependsOn: deps}
	}

	t.Run("no epic and no dependencies", func(t *testing.T) {
		i := issue("AAA-1", "AAA", 0, StatusTodo)
		all := []Issue{i, issue("AAA-2", "AAA", 0, StatusTodo)}
		if got := BlockedBy(i, all); got != nil {
			t.Fatalf("BlockedBy = %v, want nil", got)
		}
	})

	t.Run("dependency status", func(t *testing.T) {
		i := issue("AAA-1", "AAA", 0, StatusTodo, "AAA-2", "AAA-3")
		all := []Issue{
			i,
			issue("AAA-2", "AAA", 0, StatusDone),
			issue("AAA-3", "AAA", 0, StatusInProgress),
		}
		want := []string{"AAA-3"}
		if got := BlockedBy(i, all); !reflect.DeepEqual(got, want) {
			t.Fatalf("BlockedBy = %v, want %v", got, want)
		}

		all[2].Status = StatusTodo
		want = []string{"AAA-3"}
		if got := BlockedBy(i, all); !reflect.DeepEqual(got, want) {
			t.Fatalf("todo dependency: BlockedBy = %v, want %v", got, want)
		}
	})

	t.Run("dependency missing from all blocks", func(t *testing.T) {
		i := issue("AAA-1", "AAA", 0, StatusTodo, "AAA-9")
		want := []string{"AAA-9"}
		if got := BlockedBy(i, []Issue{i}); !reflect.DeepEqual(got, want) {
			t.Fatalf("BlockedBy = %v, want %v", got, want)
		}
	})

	t.Run("epic blocks only earlier epics of the same project", func(t *testing.T) {
		i := issue("AAA-5", "AAA", 2, StatusTodo)
		all := []Issue{
			i,
			issue("AAA-1", "AAA", 1, StatusTodo),
			issue("AAA-2", "AAA", 2, StatusTodo),
			issue("AAA-3", "AAA", 3, StatusTodo),
		}
		want := []string{"AAA-1"}
		if got := BlockedBy(i, all); !reflect.DeepEqual(got, want) {
			t.Fatalf("BlockedBy = %v, want %v", got, want)
		}
	})

	t.Run("epic ignores other projects", func(t *testing.T) {
		i := issue("AAA-5", "AAA", 2, StatusTodo)
		all := []Issue{
			i,
			issue("BBB-1", "BBB", 1, StatusTodo),
		}
		if got := BlockedBy(i, all); got != nil {
			t.Fatalf("BlockedBy = %v, want nil", got)
		}
	})

	t.Run("epic zero is not blocked by epics", func(t *testing.T) {
		i := issue("AAA-5", "AAA", 0, StatusTodo)
		all := []Issue{
			i,
			issue("AAA-1", "AAA", 1, StatusTodo),
			issue("AAA-2", "AAA", 2, StatusInProgress),
		}
		if got := BlockedBy(i, all); got != nil {
			t.Fatalf("BlockedBy = %v, want nil", got)
		}
	})

	t.Run("dependencies come first, then epics in all order", func(t *testing.T) {
		i := issue("AAA-9", "AAA", 3, StatusTodo, "BBB-1", "AAA-7")
		all := []Issue{
			issue("AAA-2", "AAA", 2, StatusTodo),
			issue("AAA-1", "AAA", 1, StatusTodo),
			i,
			issue("AAA-7", "AAA", 0, StatusTodo),
			issue("BBB-1", "BBB", 0, StatusTodo),
		}
		want := []string{"BBB-1", "AAA-7", "AAA-2", "AAA-1"}
		if got := BlockedBy(i, all); !reflect.DeepEqual(got, want) {
			t.Fatalf("BlockedBy = %v, want %v", got, want)
		}
	})

	t.Run("all dependencies and earlier epics done", func(t *testing.T) {
		i := issue("AAA-5", "AAA", 2, StatusTodo, "AAA-4")
		all := []Issue{
			i,
			issue("AAA-4", "AAA", 0, StatusDone),
			issue("AAA-1", "AAA", 1, StatusDone),
			issue("AAA-2", "AAA", 2, StatusTodo),
		}
		if got := BlockedBy(i, all); got != nil {
			t.Fatalf("BlockedBy = %v, want nil", got)
		}
	})

	withParent := func(id, project, parent string, epic int, status Status, deps ...string) Issue {
		return Issue{ID: id, ProjectKey: project, Parent: parent, Epic: epic, Status: status, DependsOn: deps}
	}

	t.Run("epics are counted within one goal", func(t *testing.T) {
		i := withParent("AAA-5", "AAA", "G2", 2, StatusTodo)
		all := []Issue{
			i,
			withParent("AAA-1", "AAA", "G1", 1, StatusTodo),
			withParent("AAA-2", "AAA", "G2", 1, StatusTodo),
		}
		want := []string{"AAA-2"}
		if got := BlockedBy(i, all); !reflect.DeepEqual(got, want) {
			t.Fatalf("BlockedBy = %v, want %v", got, want)
		}
	})

	t.Run("an issue with a parent is not blocked by an epic without a parent", func(t *testing.T) {
		i := withParent("AAA-5", "AAA", "G1", 2, StatusTodo)
		all := []Issue{
			i,
			issue("AAA-1", "AAA", 1, StatusTodo),
		}
		if got := BlockedBy(i, all); got != nil {
			t.Fatalf("BlockedBy = %v, want nil", got)
		}
	})

	t.Run("a goal is blocked by its unfinished parts only", func(t *testing.T) {
		goal := withParent("AAA-1", "AAA", "", 0, StatusTodo)
		all := []Issue{
			goal,
			withParent("AAA-2", "AAA", "AAA-1", 0, StatusDone),
			withParent("AAA-3", "AAA", "AAA-1", 0, StatusTodo),
		}
		want := []string{"AAA-3"}
		if got := BlockedBy(goal, all); !reflect.DeepEqual(got, want) {
			t.Fatalf("BlockedBy = %v, want %v", got, want)
		}

		all[2].Status = StatusDone
		if got := BlockedBy(goal, all); got != nil {
			t.Fatalf("all parts done: BlockedBy = %v, want nil", got)
		}
	})

	t.Run("a goal's dependency on an unfinished part is listed once", func(t *testing.T) {
		goal := withParent("AAA-1", "AAA", "", 0, StatusTodo, "AAA-3")
		all := []Issue{
			goal,
			withParent("AAA-3", "AAA", "AAA-1", 0, StatusTodo),
		}
		want := []string{"AAA-3"}
		if got := BlockedBy(goal, all); !reflect.DeepEqual(got, want) {
			t.Fatalf("BlockedBy = %v, want %v", got, want)
		}
	})
}
