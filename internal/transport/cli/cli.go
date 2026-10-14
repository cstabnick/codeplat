// Package cli exposes core.Service as command-line subcommands.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"codeplat/internal/core"
)

const usage = `usage:
  codeplat project create KEY NAME
  codeplat project list
  codeplat issue create -project KEY -title TITLE [-desc TEXT] [-assignee NAME] [-stage STAGE] [-epic N] [-depends-on IDS]
  codeplat issue list [-project KEY] [-stage STAGE] [-status STATUS] [-assignee NAME] [-ready] [-limit N] [-offset N]
  codeplat issue show ID
  codeplat issue update ID [-title TITLE] [-desc TEXT] [-assignee NAME] [-stage STAGE] [-status STATUS] [-epic N] [-depends-on IDS]
  codeplat issue claim ID -assignee NAME
  codeplat issue block ID -author NAME -reason TEXT
  codeplat issue comment ID -body TEXT [-author NAME]
  codeplat issue comments ID [-limit N] [-offset N]

STAGE is one of: refinement, implementation, testing
STATUS is one of: todo, in_progress, done, blocked
IDS is a comma-separated list of issue IDs.
limit defaults to 50 and may be at most 200; offset skips that many
`

var errUsage = errors.New("usage")

// Run executes one command and returns the process exit code.
func Run(ctx context.Context, svc *core.Service, args []string, stdout, stderr io.Writer) int {
	c := &command{svc: svc, out: stdout, errOut: stderr}
	err := c.dispatch(ctx, args)
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		fmt.Fprint(stderr, usage)
		return 2
	default:
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
}

type command struct {
	svc    *core.Service
	out    io.Writer
	errOut io.Writer
}

func (c *command) dispatch(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return errUsage
	}
	rest := args[2:]
	switch args[0] + " " + args[1] {
	case "project create":
		return c.projectCreate(ctx, rest)
	case "project list":
		return c.projectList(ctx)
	case "issue create":
		return c.issueCreate(ctx, rest)
	case "issue list":
		return c.issueList(ctx, rest)
	case "issue show":
		return c.issueShow(ctx, rest)
	case "issue update":
		return c.issueUpdate(ctx, rest)
	case "issue claim":
		return c.issueClaim(ctx, rest)
	case "issue block":
		return c.issueBlock(ctx, rest)
	case "issue comment":
		return c.issueComment(ctx, rest)
	case "issue comments":
		return c.issueComments(ctx, rest)
	default:
		return errUsage
	}
}

func (c *command) projectCreate(ctx context.Context, args []string) error {
	if len(args) != 2 {
		return errUsage
	}
	p, err := c.svc.CreateProject(ctx, core.NewProject{Key: args[0], Name: args[1]})
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "created project %s\n", p.Key)
	return nil
}

func (c *command) projectList(ctx context.Context) error {
	projects, err := c.svc.ListProjects(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tNAME")
	for _, p := range projects {
		fmt.Fprintf(tw, "%s\t%s\n", p.Key, p.Name)
	}
	return tw.Flush()
}

func (c *command) issueCreate(ctx context.Context, args []string) error {
	var in core.NewIssue
	fs := c.flagSet("issue create")
	fs.StringVar(&in.ProjectKey, "project", "", "project key")
	fs.StringVar(&in.Title, "title", "", "title")
	fs.StringVar(&in.Description, "desc", "", "description")
	fs.StringVar(&in.Assignee, "assignee", "", "assignee")
	fs.StringVar((*string)(&in.Stage), "stage", "", "stage")
	fs.IntVar(&in.Epic, "epic", 0, "epic number, 0 for none")
	var dependsOn string
	fs.StringVar(&dependsOn, "depends-on", "", "comma-separated issue IDs this one waits for")
	if err := parse(fs, args); err != nil {
		return err
	}
	in.DependsOn = splitIDs(dependsOn)
	i, err := c.svc.CreateIssue(ctx, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "created issue %s\n", i.ID)
	return nil
}

func (c *command) issueList(ctx context.Context, args []string) error {
	var f core.IssueFilter
	fs := c.flagSet("issue list")
	fs.StringVar(&f.ProjectKey, "project", "", "project key")
	fs.StringVar((*string)(&f.Stage), "stage", "", "stage")
	fs.StringVar((*string)(&f.Status), "status", "", "status")
	fs.StringVar(&f.Assignee, "assignee", "", "assignee")
	fs.BoolVar(&f.Ready, "ready", false, "only issues that nothing blocks")
	fs.IntVar(&f.Limit, "limit", 0, "at most this many issues (default 50, max 200)")
	fs.IntVar(&f.Offset, "offset", 0, "skip this many issues")
	if err := parse(fs, args); err != nil {
		return err
	}
	issues, err := c.svc.ListIssues(ctx, f)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTAGE\tSTATUS\tASSIGNEE\tTITLE")
	for _, i := range issues {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", i.ID, i.Stage, i.Status, i.Assignee, i.Title)
	}
	return tw.Flush()
}

func (c *command) issueShow(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errUsage
	}
	i, err := c.svc.GetIssue(ctx, args[0])
	if err != nil {
		return err
	}
	c.printIssue(i)
	return nil
}

func (c *command) issueUpdate(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errUsage
	}
	var title, desc, assignee, stage, status, dependsOn string
	var epic int
	fs := c.flagSet("issue update")
	fs.StringVar(&title, "title", "", "title")
	fs.StringVar(&desc, "desc", "", "description")
	fs.StringVar(&assignee, "assignee", "", "assignee")
	fs.StringVar(&stage, "stage", "", "stage")
	fs.StringVar(&status, "status", "", "status")
	fs.IntVar(&epic, "epic", 0, "epic number, 0 for none")
	fs.StringVar(&dependsOn, "depends-on", "", "comma-separated issue IDs this one waits for")
	if err := parse(fs, args[1:]); err != nil {
		return err
	}
	// Only flags that were actually passed change the issue, so
	// `-assignee ""` clears the assignee while omitting it leaves it alone.
	var patch core.IssuePatch
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "title":
			patch.Title = &title
		case "desc":
			patch.Description = &desc
		case "assignee":
			patch.Assignee = &assignee
		case "stage":
			s := core.Stage(stage)
			patch.Stage = &s
		case "status":
			s := core.Status(status)
			patch.Status = &s
		case "epic":
			patch.Epic = &epic
		case "depends-on":
			deps := splitIDs(dependsOn)
			patch.DependsOn = &deps
		}
	})
	i, err := c.svc.UpdateIssue(ctx, args[0], patch)
	if err != nil {
		return err
	}
	c.printIssue(i)
	return nil
}

func (c *command) issueClaim(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errUsage
	}
	var assignee string
	fs := c.flagSet("issue claim")
	fs.StringVar(&assignee, "assignee", "", "assignee")
	if err := parse(fs, args[1:]); err != nil {
		return err
	}
	i, err := c.svc.ClaimIssue(ctx, args[0], assignee)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "%s claimed %s\n", i.Assignee, i.ID)
	return nil
}

func (c *command) issueBlock(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errUsage
	}
	var author, reason string
	fs := c.flagSet("issue block")
	fs.StringVar(&author, "author", "", "who is blocked")
	fs.StringVar(&reason, "reason", "", "why the issue cannot be finished")
	if err := parse(fs, args[1:]); err != nil {
		return err
	}
	i, err := c.svc.BlockIssue(ctx, args[0], author, reason)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "blocked %s\n", i.ID)
	return nil
}

func (c *command) issueComment(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errUsage
	}
	in := core.NewComment{IssueID: args[0]}
	fs := c.flagSet("issue comment")
	fs.StringVar(&in.Body, "body", "", "comment text")
	fs.StringVar(&in.Author, "author", "", "author")
	if err := parse(fs, args[1:]); err != nil {
		return err
	}
	cm, err := c.svc.AddComment(ctx, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "added comment #%d to %s\n", cm.Number, cm.IssueID)
	return nil
}

func (c *command) issueComments(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errUsage
	}
	var p core.Page
	fs := c.flagSet("issue comments")
	fs.IntVar(&p.Limit, "limit", 0, "at most this many comments (default 50, max 200)")
	fs.IntVar(&p.Offset, "offset", 0, "skip this many comments")
	if err := parse(fs, args[1:]); err != nil {
		return err
	}
	comments, err := c.svc.ListCommentsPage(ctx, args[0], p)
	if err != nil {
		return err
	}
	for n, cm := range comments {
		if n > 0 {
			fmt.Fprintln(c.out)
		}
		author := cm.Author
		if author == "" {
			author = "(no author)"
		}
		fmt.Fprintf(c.out, "#%d  %s  %s\n%s\n", cm.Number, author, cm.CreatedAt.Format("2006-01-02 15:04:05Z07:00"), cm.Body)
	}
	return nil
}

func (c *command) printIssue(i core.Issue) {
	deps := "-"
	if len(i.DependsOn) > 0 {
		deps = strings.Join(i.DependsOn, ", ")
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID\t%s\n", i.ID)
	fmt.Fprintf(tw, "Title\t%s\n", i.Title)
	fmt.Fprintf(tw, "Stage\t%s\n", i.Stage)
	fmt.Fprintf(tw, "Status\t%s\n", i.Status)
	fmt.Fprintf(tw, "Epic\t%d\n", i.Epic)
	fmt.Fprintf(tw, "Depends on\t%s\n", deps)
	fmt.Fprintf(tw, "Assignee\t%s\n", i.Assignee)
	fmt.Fprintf(tw, "Created\t%s\n", i.CreatedAt.Format("2006-01-02 15:04:05Z07:00"))
	fmt.Fprintf(tw, "Updated\t%s\n", i.UpdatedAt.Format("2006-01-02 15:04:05Z07:00"))
	tw.Flush()
	if i.Description != "" {
		fmt.Fprintf(c.out, "\n%s\n", i.Description)
	}
}

// splitIDs splits a comma-separated list of issue IDs. It trims each
// item and drops empty ones. It returns a non-nil slice.
func splitIDs(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		if id := strings.TrimSpace(part); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func (c *command) flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return errUsage
	}
	return nil
}
