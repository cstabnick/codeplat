package core

// BlockedBy returns the IDs that keep issue from being ready, in this
// order, with each ID listed once:
//
//  1. each ID in issue.DependsOn whose issue is not done, or is not in
//     all at all;
//  2. when issue.Epic > 0, the ID of each issue in all with the same
//     ProjectKey and the same Parent, 0 < Epic < issue.Epic, and Status
//     other than StatusDone, in the order they appear in all;
//  3. the ID of each issue in all whose Parent is issue.ID and whose
//     Status is not StatusDone, in the order they appear in all.
//
// It returns nil when nothing blocks the issue. all may contain issues
// of several projects and goals. Epics are counted only within one goal
// (Parent) of one project, so two goals do not block each other. An
// issue with Epic 0 is never blocked by epics, and never blocks one. A
// goal (an issue that other issues name as Parent) is blocked by its
// parts that are not done.
func BlockedBy(issue Issue, all []Issue) []string {
	done := make(map[string]bool, len(all))
	for _, other := range all {
		done[other.ID] = other.Status == StatusDone
	}

	var blockers []string
	seen := make(map[string]bool)
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			blockers = append(blockers, id)
		}
	}

	for _, dep := range issue.DependsOn {
		if !done[dep] {
			add(dep)
		}
	}

	if issue.Epic > 0 {
		for _, other := range all {
			if other.ProjectKey != issue.ProjectKey || other.Parent != issue.Parent {
				continue
			}
			if other.Epic <= 0 || other.Epic >= issue.Epic {
				continue
			}
			if other.Status != StatusDone {
				add(other.ID)
			}
		}
	}

	for _, other := range all {
		if other.Parent == issue.ID && other.Status != StatusDone {
			add(other.ID)
		}
	}

	return blockers
}
