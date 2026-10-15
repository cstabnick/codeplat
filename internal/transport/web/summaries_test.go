package web

import (
	"testing"
	"time"

	"codeplat/internal/core"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func issue(project string, status core.Status, updated time.Time) core.Issue {
	return core.Issue{ProjectKey: project, Status: status, UpdatedAt: updated}
}

func keys(list []projectInfo) []string {
	out := make([]string, len(list))
	for i, p := range list {
		out[i] = p.Key
	}
	return out
}

func equalKeys(a, b []string) bool {
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

func TestSummariesCounts(t *testing.T) {
	projects := []core.Project{
		{Key: "AAA", Name: "Alpha", CreatedAt: t0},
		{Key: "BBB", Name: "Beta", CreatedAt: t0},
	}
	issues := []core.Issue{
		issue("AAA", core.StatusTodo, t0.Add(1*time.Hour)),
		issue("AAA", core.StatusInProgress, t0.Add(2*time.Hour)),
		issue("AAA", core.StatusBlocked, t0.Add(3*time.Hour)),
		issue("AAA", core.StatusDone, t0.Add(4*time.Hour)),
		issue("BBB", core.StatusDone, t0.Add(5*time.Hour)),
	}
	got := summaries(projects, issues)
	if len(got) != 2 {
		t.Fatalf("got %d summaries; want 2", len(got))
	}
	a := got[0]
	if a.Key != "AAA" || a.Open != 3 || a.Blocked != 1 || a.Done != 1 || a.Total != 4 {
		t.Errorf("AAA = %+v; want Open 3, Blocked 1, Done 1, Total 4", a)
	}
	if want := t0.Add(4 * time.Hour); !a.Updated.Equal(want) {
		t.Errorf("AAA Updated = %v; want %v", a.Updated, want)
	}
	b := got[1]
	if b.Key != "BBB" || b.Open != 0 || b.Blocked != 0 || b.Done != 1 || b.Total != 1 {
		t.Errorf("BBB = %+v; want Open 0, Blocked 0, Done 1, Total 1", b)
	}
	if want := "alpha aaa"; a.Filter != want {
		t.Errorf("AAA Filter = %q; want %q", a.Filter, want)
	}
}

func TestSummariesOrder(t *testing.T) {
	projects := []core.Project{
		{Key: "B1", Name: "beta"},
		{Key: "A1", Name: "Alpha"},
		{Key: "A0", Name: "alpha"},
	}
	got := keys(summaries(projects, nil))
	if want := []string{"A0", "A1", "B1"}; !equalKeys(got, want) {
		t.Errorf("order = %v; want %v (lower-cased name, then key)", got, want)
	}
}

func TestSummariesUpdatedFallsBackToCreatedAt(t *testing.T) {
	created := t0.Add(-48 * time.Hour)
	projects := []core.Project{
		{Key: "AAA", Name: "Alpha", CreatedAt: created},
		{Key: "BBB", Name: "Beta", CreatedAt: created},
	}
	got := summaries(projects, []core.Issue{issue("AAA", core.StatusTodo, t0)})
	for _, p := range got {
		switch p.Key {
		case "AAA":
			if !p.Updated.Equal(t0) {
				t.Errorf("AAA Updated = %v; want its issue's %v", p.Updated, t0)
			}
		case "BBB":
			if !p.Updated.Equal(created) {
				t.Errorf("BBB Updated = %v; want its CreatedAt %v", p.Updated, created)
			}
		}
	}
}

func TestSummariesIgnoresUnknownProjects(t *testing.T) {
	projects := []core.Project{{Key: "AAA", Name: "Alpha", CreatedAt: t0}}
	issues := []core.Issue{
		issue("ZZZ", core.StatusTodo, t0.Add(time.Hour)),
		issue("ZZZ", core.StatusDone, t0.Add(2*time.Hour)),
	}
	got := summaries(projects, issues)
	if len(got) != 1 {
		t.Fatalf("got %d summaries; want 1", len(got))
	}
	if p := got[0]; p.Total != 0 || p.Open != 0 || p.Done != 0 || !p.Updated.Equal(t0) {
		t.Errorf("AAA = %+v; want no counts and Updated at CreatedAt", p)
	}
}

func mk(key string, updated time.Time) projectInfo {
	return projectInfo{Key: key, Name: key, Updated: updated}
}

// eight projects, E0 the most recently updated and H7 the least
func eight() []projectInfo {
	var out []projectInfo
	for i := 0; i < 8; i++ {
		out = append(out, mk("P"+string(rune('0'+i)), t0.Add(time.Duration(8-i)*time.Hour)))
	}
	return out
}

func TestTabsEmpty(t *testing.T) {
	if got := tabs(nil, ""); got != nil {
		t.Errorf("tabs(nil) = %v; want nil", got)
	}
	if got := tabs([]projectInfo{}, "X"); got != nil {
		t.Errorf("tabs(empty, active) = %v; want nil", got)
	}
}

func TestTabsCappedAndOrdered(t *testing.T) {
	got := keys(tabs(eight(), ""))
	if want := []string{"P0", "P1", "P2", "P3", "P4", "P5"}; !equalKeys(got, want) {
		t.Errorf("tabs = %v; want %v (most recent first, capped at %d)", got, want, maxTabs)
	}
}

func TestTabsTiesBrokenByKey(t *testing.T) {
	same := t0
	all := []projectInfo{mk("C", same), mk("A", same), mk("B", same)}
	got := keys(tabs(all, ""))
	if want := []string{"A", "B", "C"}; !equalKeys(got, want) {
		t.Errorf("tabs = %v; want %v", got, want)
	}
}

func TestTabsActiveReplacesLast(t *testing.T) {
	all := eight()
	got := tabs(all, "P7") // the least recently updated
	if len(got) != maxTabs {
		t.Fatalf("got %d tabs; want %d", len(got), maxTabs)
	}
	if want := []string{"P0", "P1", "P2", "P3", "P4", "P7"}; !equalKeys(keys(got), want) {
		t.Errorf("tabs = %v; want %v", keys(got), want)
	}
	if last := got[len(got)-1]; last.Key != "P7" {
		t.Errorf("last tab = %q; want the active P7", last.Key)
	}
	if !got[len(got)-1].Updated.Equal(all[7].Updated) {
		t.Error("active tab lost its Updated time")
	}
}

func TestTabsActiveAlreadyShownKeepsList(t *testing.T) {
	got := keys(tabs(eight(), "P2"))
	if want := []string{"P0", "P1", "P2", "P3", "P4", "P5"}; !equalKeys(got, want) {
		t.Errorf("tabs = %v; want %v unchanged", got, want)
	}
}

func TestTabsUnknownActive(t *testing.T) {
	got := keys(tabs(eight(), "NOPE"))
	if want := []string{"P0", "P1", "P2", "P3", "P4", "P5"}; !equalKeys(got, want) {
		t.Errorf("tabs with unknown active = %v; want %v", got, want)
	}
}

func TestTabsFewerThanCapShowsAll(t *testing.T) {
	all := []projectInfo{mk("A", t0), mk("B", t0.Add(time.Hour))}
	got := keys(tabs(all, "A"))
	if want := []string{"B", "A"}; !equalKeys(got, want) {
		t.Errorf("tabs = %v; want %v", got, want)
	}
}
