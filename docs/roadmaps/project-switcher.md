# Roadmap: a project switcher for the web UI

## Goal

The web UI stops using one tab per project. The header keeps up to six tabs (the most recently changed projects plus the selected one) and gains a switcher: a button that opens a filterable list of every project with its open count. A `/projects` page lists every project in a table with its counts. On a phone only the selected tab shows. With a hundred projects the header looks the same as with three.

Tracker project: `PLAT`.

## Why

`internal/transport/web/page.html` renders `{{range .Projects}}` as one tab each in a header that scrolls sideways (`overflow-x: auto`). With a dozen projects the active one can be off-screen and there is no way to find another. Projects are the top-level unit of the tracker, so the number only grows (the repository already has `PLAT`, `BOOK`, `LLM` and `CHAT`).

## Design

- **Tabs stay, but they are a shortcut.** At most `maxTabs` (6): the most recently updated projects, and the selected project always, replacing the last one if it is not among them. "Updated" is the newest `UpdatedAt` of the project's issues, or the project's `CreatedAt` when it has none. Tabs are not reordered by name, so the shortcut tracks where work is happening.
- **Switcher.** A `<details>` element in the header (it opens and closes without JavaScript) holding a filter box and every project by name with its open count. A small inline script filters the list as the person types and focuses the box when it opens; Escape closes it. The script listens at document level so it keeps working after htmx swaps the page.
- **Projects page.** `GET /projects`: a table with Project, Open, Blocked, Done, Total and Last change, and the same filter box.
- **Counts without a new use case.** The tracker has no per-project count query, and a new use case would have to be added to REST, CLI and MCP too. The web layer already reads every page of issues with `all`; it reads all issues once per request and groups them by project, so the cost is one pass however many projects there are. The issue page already reads every issue, so it reuses that read.
- **Narrow screens.** Below 640px only the selected tab shows, next to the switcher.

Not included: pinning or reordering tabs by hand, remembering recent projects in the browser, and counts that update live (they refresh when the page is loaded; the board itself stays live).

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Data | The handlers compute per-project counts, a tab list and a `/projects` route; the page still renders as before. | 1 |
| 2. Switcher | The header shows up to six tabs and a filterable switcher, and `/projects` lists every project. | 1 |
| 3. Docs | `ARCHITECTURE.md` describes the switcher. | 1 |

No issue has a dependency inside its epic.

Why three epics of one issue each: epics 1 and 2 both edit `web.go`, and the template of epic 2 renders fields that epic 1 creates, so they cannot run side by side (technique: move the shared foundation to an earlier epic). Epic 1 leaves the page untouched, so the existing tests keep passing between the two. The docs describe behaviour that must exist first, so they come last, after an earlier plan's docs issue was blocked for describing a board that did not exist yet.

---

## Epic 1: Data

Done when: The web handlers compute per-project counts, a tab list and a `/projects` route; the page still renders as before.

### 1.1 Compute project summaries and add a projects page route

~~~~text
Epic: 1 Data
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - The web UI (`internal/transport/web`) is read-only. Business rules live in `internal/core`; do not add any to the web package, and do not change `internal/core`.
  - Write tests with `apptest.New(t)` (see `internal/transport/web/web_test.go`); do not construct stores or the service by hand.
- Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
- The web server may be running from `bin/codeplat-web` while you work. Do not stop or restart it.
- Another worker edits `internal/transport/web/page.html` in the next epic. Do not edit it: the page must render exactly as it does now.

## Goal
Give the web handlers what a scalable project switcher needs: for every project, its issue counts and when it last changed, a short list of projects to show as tabs, and a `GET /projects` route. The page does not change yet; the next epic renders the data.

## Read first
- `internal/transport/web/web.go`: the `page` struct, `home`, `project`, `issue` handlers (each calls `h.svc.ListProjects` and passes `Projects: projects` to `h.render`), the `all` helper at the bottom (`all(func(core.Page) ([]T, error)) ([]T, error)` reads every page of a list), and `NewHandler`'s mux.
- `internal/core/types.go`: `Project{Key, Name, CreatedAt}`, `Issue` (`ProjectKey`, `Status`, `UpdatedAt`) and the statuses `core.StatusTodo`, `core.StatusInProgress`, `core.StatusDone`, `core.StatusBlocked`.
- `internal/transport/web/web_test.go`: how tests build projects and issues with `apptest.New(t)`.

## Change
- `internal/transport/web/web.go`:
  - Update the package comment's first sentence to: `Package web is a read-only browser UI over core.Service: a project switcher, a board of the selected project's issues by goal, epic and stage, and a page per issue.` (keep the rest of the comment).
  - Add

        // maxTabs is how many project tabs the header shows. Every project stays
        // reachable through the switcher and the /projects page.
        const maxTabs = 6

        // projectInfo is a project with counts of its issues.
        type projectInfo struct {
        	Key     string
        	Name    string
        	Open    int       // issues whose status is not done
        	Blocked int       // issues whose status is blocked; also counted in Open
        	Done    int       // issues whose status is done
        	Total   int
        	Updated time.Time // when an issue of the project last changed; the project's CreatedAt if it has none
        	Filter  string    // strings.ToLower(Name + " " + Key), for the page's filter box
        }

  - Add to the `page` struct (keep the existing `Projects []core.Project` field and set it as now):

        Summaries []projectInfo // every project, ordered by lower-cased Name then Key
        Tabs      []projectInfo // the projects shown as header tabs
        Index     bool          // render the projects page instead of a board

  - Add the pure functions (unexported):

        // summaries counts each project's issues. The result has one entry per
        // project, ordered by strings.ToLower(Name), then Key.
        func summaries(projects []core.Project, issues []core.Issue) []projectInfo

        // tabs picks the projects to show as tabs: the maxTabs most recently
        // updated (ties broken by Key), plus the active project if it is not among
        // them, which then replaces the last of them. active may be "" or unknown.
        // The result is ordered most recently updated first, and is nil for no projects.
        func tabs(all []projectInfo, active string) []projectInfo

    An issue whose `ProjectKey` is not in `projects` is ignored. `Open` counts every issue whose status is not done (todo, in_progress, blocked).
  - Add a method `func (h *handler) summaries(ctx context.Context, projects []core.Project) ([]projectInfo, error)` that reads every issue with `all(func(p core.Page) ([]core.Issue, error) { return h.svc.ListIssues(ctx, core.IssueFilter{Page: p}) })` and returns `summaries(projects, issues)`.
  - In the `project` and `issue` handlers, and in their not-found renders, set `Summaries` and `Tabs` (`tabs(summaries, activeKey)`, where the active key is the project key, the issue's project key, or "" in the two not-found cases). In `issue`, the handler already reads every issue into `others`: build the summaries from `others` instead of calling `h.summaries`, so it still reads the issues once. An error from `h.summaries` goes to `h.fail`.
  - Add the route `mux.HandleFunc("GET /projects", h.projectsIndex)` and

        // projectsIndex renders the page listing every project with its counts.
        func (h *handler) projectsIndex(w http.ResponseWriter, r *http.Request)

    It renders `page{Title: "Projects", Projects: projects, Summaries: s, Tabs: tabs(s, ""), Index: true}` with status 200, also when there are no projects.
  - `GET /projects/{key}` is unchanged and still wins for paths under `/projects/`.
- Do not change `page.html`.

## Contract
```go
const maxTabs = 6

type projectInfo struct {
	Key     string
	Name    string
	Open    int
	Blocked int
	Done    int
	Total   int
	Updated time.Time
	Filter  string
}

func summaries(projects []core.Project, issues []core.Issue) []projectInfo
func tabs(all []projectInfo, active string) []projectInfo
func (h *handler) projectsIndex(w http.ResponseWriter, r *http.Request)
```
`page` gains `Summaries []projectInfo`, `Tabs []projectInfo`, `Index bool`. Route `GET /projects`.

## Done when
- `go test ./internal/transport/web/` passes.
- `go vet ./internal/transport/web/` succeeds and `gofmt -l internal/transport/web` prints nothing.
- `go build -o /dev/null ./cmd/web` succeeds.

## Out of scope
- `internal/transport/web/page.html` and every test file; the testing worker adds the tests (a new `internal/transport/web/summaries_test.go` in package `web` for `summaries` and `tabs`, and `/projects` cases in `web_test.go`).
- `internal/core`, the other transports, the docs.
~~~~

## Epic 2: Switcher

Done when: The header shows up to six tabs and a filterable switcher, and `/projects` lists every project.

### 2.1 Show a project switcher and a projects page in the web UI

~~~~text
Epic: 2 Switcher
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - The web UI (`internal/transport/web`) is read-only. Business rules live in `internal/core`; do not add any to the web package, and do not change `internal/core`.
  - Write tests with `apptest.New(t)` (see `internal/transport/web/web_test.go`); do not construct stores or the service by hand.
- Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
- The web server may be running from `bin/codeplat-web` while you work. Do not stop or restart it.

## Goal
Make the header scale to any number of projects. Keep a few tabs for the active and most recently changed projects, and add a switcher: a button that opens a filterable list of every project with its open count, and a `/projects` page with a table of all projects. A phone shows only the selected tab.

## Read first
- `internal/transport/web/web.go`: the `page` struct, which now has `Summaries []projectInfo`, `Tabs []projectInfo` and `Index bool` next to the old `Projects []core.Project`; `projectInfo` (`Key`, `Name`, `Open`, `Blocked`, `Done`, `Total`, `Updated time.Time`, `Filter string`) and the template function map (`label`, `ts`).
- `internal/transport/web/page.html`: the `<style>` block (`header`, `.brand`, `.tabs`, `.tab`), the `<header>` markup with `{{range .Projects}}` tabs, and the `{{if .Board}}` / `{{if .Issue}}` sections in `<main>`.
- `internal/transport/web/web_test.go`: the assertions about tabs (`<a class="tab active" href="/projects/AAA">Alpha` and `<a class="tab" href="/projects/BBB">Beta`). Tabs keep that exact markup, so they keep passing.

## Change
- `internal/transport/web/page.html`:
  1. Header markup. Replace the tabs loop with `{{range .Tabs}}` (same `<a class="tab{{if eq .Key $.Active}} active{{end}}" href="/projects/{{.Key}}">{{.Name}}<small>{{.Key}}</small></a>`), and after the `<nav class="tabs">` add, only when `{{if .Summaries}}`:

         <details class="switcher">
           <summary>All projects <small>{{len .Summaries}}</small></summary>
           <div class="panel">
             <input id="project-filter" type="search" placeholder="Filter projects…" autocomplete="off" data-filter-list="#switcher-list">
             <ul id="switcher-list" class="project-list">
               {{range .Summaries}}<li data-filter="{{.Filter}}"><a class="project-link{{if eq .Key $.Active}} active{{end}}" href="/projects/{{.Key}}"><span class="pname">{{.Name}}</span><small>{{.Key}}</small><span class="pcount">{{.Open}} open</span></a></li>
               {{end}}
             </ul>
             <a class="all-link" href="/projects">Open the projects page</a>
           </div>
         </details>
  2. Projects page. In `<main>`, add `{{if .Index}}` … `{{end}}` with: an `<h1>Projects</h1>`; an `<input id="index-filter" type="search" placeholder="Filter projects…" autocomplete="off" data-filter-list="#project-rows">`; and, when `.Summaries` is not empty, a `<table class="projects">` whose `<thead>` has the columns Project, Open, Blocked, Done, Total, Last change, and whose `<tbody id="project-rows">` has one `<tr data-filter="{{.Filter}}">` per summary: `<td><a href="/projects/{{.Key}}">{{.Name}}</a> <small>{{.Key}}</small></td>`, then `.Open`, `.Blocked`, `.Done`, `.Total`, and `{{ts .Updated}}`. With no summaries show `<p class="empty">No projects yet.</p>` instead of the filter and table. Wrap it all in `<section id="projects">`.
  3. Filter script. In `<head>`, after the htmx scripts, add one inline `<script>` (plain JavaScript) that, at document level so it still works after htmx swaps the page:
     - on `input` events whose target has a `data-filter-list` attribute, lower-cases and trims the target's value and sets `hidden` on every `[data-filter]` element inside the element matched by that attribute's selector, to `true` when the value is not empty and the element's `data-filter` does not contain it;
     - on `toggle` events (use capture, they do not bubble) from a `details.switcher` that has just opened, focuses `#project-filter`;
     - on `keydown` of `Escape` while a `details.switcher` is open, closes it.
  4. CSS in the existing `<style>` block, using the existing variables: `header` gets `overflow: visible` (it had `overflow-x: auto`, which would clip the panel) and `.tabs` gets `min-width: 0; overflow-x: auto`; `.switcher { position: relative; margin-left: auto; padding-bottom: 8px; }`; `.switcher > summary` is a button-like pill (border `1px solid var(--line)`, radius, padding, `cursor: pointer`, `list-style: none`, `white-space: nowrap`) with `small` muted; `.panel { position: absolute; right: 0; top: 100%; z-index: 10; width: min(420px, calc(100vw - 32px)); max-height: 70vh; overflow-y: auto; background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 10px; box-shadow: 0 8px 24px rgba(0,0,0,.18); }`; the filter inputs are full width with the page's border and radius; `.project-list { list-style: none; margin: 8px 0; padding: 0; }`; `.project-link { display: flex; gap: 8px; align-items: baseline; padding: 6px 8px; border-radius: 6px; }`, with `.pname { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }`, `.pcount` muted, `.project-link:hover` and `.project-link.active` highlighted; `table.projects { width: 100%; border-collapse: collapse; background: var(--panel); border: 1px solid var(--line); }` with padded cells, `border-bottom: 1px solid var(--line)`, left-aligned first column and right-aligned number columns; `[hidden] { display: none !important; }`; and `@media (max-width: 640px) { .tab:not(.active) { display: none; } }`.
- `internal/transport/web/web.go`: remove the `Projects []core.Project` field from `page` and the `Projects: projects` argument from every `page{...}` literal (the not-found renders, `project`, `issue`, `projectsIndex`); the template no longer uses it. The `home` handler keeps its own `projects` variable for the redirect.

## Contract
Element ids and classes: `switcher` (class on the `<details>`), `project-filter`, `switcher-list`, `project-list`, `project-link`, `all-link`, `projects` (section), `index-filter`, `project-rows`; attributes `data-filter`, `data-filter-list`. Route `GET /projects` (already exists). The tab markup is unchanged.

## Done when
- `go test ./internal/transport/web/` passes.
- `go vet ./internal/transport/web/` succeeds and `gofmt -l internal/transport/web` prints nothing.
- `grep -c 'range .Projects' internal/transport/web/page.html` prints 0.
- `grep -c 'Projects:' internal/transport/web/web.go` prints 0.

## Out of scope
- `internal/core`, the other transports and every doc file; the next epic updates `ARCHITECTURE.md`.
- Test files; the testing worker adds cases to `web_test.go`: with eight projects only the six most recent are tabs and the selected one is always a tab, all eight are in the switcher list, `/projects` lists all eight with counts, names with HTML are escaped in the tabs, the switcher and the table, and the page has one `data-filter-list` script.
~~~~

## Epic 3: Docs

Done when: `ARCHITECTURE.md` describes the switcher.

### 3.1 Describe the project switcher in ARCHITECTURE.md

~~~~text
Epic: 3 Docs
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - The web UI (`internal/transport/web`) is read-only. Business rules live in `internal/core`; do not add any to the web package, and do not change `internal/core`.
  - Write tests with `apptest.New(t)` (see `internal/transport/web/web_test.go`); do not construct stores or the service by hand.
- Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
- The web server may be running from `bin/codeplat-web` while you work. Do not stop or restart it.

## Goal
Make `ARCHITECTURE.md` say what the web UI does now, so the next reader does not look for one tab per project.

## Read first
- `ARCHITECTURE.md`: the **Web UI** bullet in the Decisions section. It currently says "a tab per project, a board grouped by goal and epic with a column per stage, a page per issue."
- `internal/transport/web/web.go`: `maxTabs`, `projectInfo`, `tabs`, `summaries` and the `GET /projects` route, to describe them exactly.
- `internal/transport/web/page.html`: the `switcher` markup and the filter script.

## Change
- `ARCHITECTURE.md`: in the **Web UI** bullet replace "a tab per project" with a description in the same register: the header shows at most six project tabs (the most recently changed, plus the selected one) and a switcher with a filter box that lists every project with its open count; `/projects` is a page that lists every project with its counts; the counts and the order come from reading every issue once per request (`summaries`), so any number of projects stays reachable. Keep the rest of the bullet. Do not edit any other bullet.
- Do not edit any other file.

## Contract
File path `ARCHITECTURE.md`.

## Done when
- `grep -c 'a tab per project' ARCHITECTURE.md` prints 0.
- `grep -c 'switcher' ARCHITECTURE.md` prints at least 1.
- `grep -c '/projects' ARCHITECTURE.md` prints at least 1.

## Out of scope
- Every Go and HTML file.
~~~~
