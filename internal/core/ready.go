package core

// BlockedBy returns the IDs that keep issue from being ready, in this
// order: each ID in issue.DependsOn whose issue is not done, or is not
// in all at all; then the ID of each issue in all with the same
// ProjectKey, 0 < Epic < issue.Epic, and Status other than StatusDone,
// in the order they appear in all. It returns nil when nothing blocks
// the issue. all may contain issues of several projects. An issue with
// Epic 0 is never blocked by epics, and never blocks one.
func BlockedBy(issue Issue, all []Issue) []string {
	done := make(map[string]bool, len(all))
	for _, other := range all {
		done[other.ID] = other.Status == StatusDone
	}

	var blockers []string
	for _, dep := range issue.DependsOn {
		if !done[dep] {
			blockers = append(blockers, dep)
		}
	}

	if issue.Epic > 0 {
		for _, other := range all {
			if other.ProjectKey != issue.ProjectKey {
				continue
			}
			if other.Epic <= 0 || other.Epic >= issue.Epic {
				continue
			}
			if other.Status != StatusDone {
				blockers = append(blockers, other.ID)
			}
		}
	}

	return blockers
}
