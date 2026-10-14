// Package cli exposes core.Service as command-line subcommands.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"codeplat/internal/core"
)

const usage = `usage:
  codeplat project create KEY NAME
  codeplat project list
  codeplat issue create -project KEY -title TITLE [-desc TEXT] [-assignee NAME]
  codeplat issue list [-project KEY] [-status STATUS] [-assignee NAME]
  codeplat issue show ID
  codeplat issue update ID [-title TITLE] [-desc TEXT] [-assignee NAME] [-status STATUS]

STATUS is one of: todo, in_progress, done
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
	if err := parse(fs, args); err != nil {
		return err
	}
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
	fs.StringVar((*string)(&f.Status), "status", "", "status")
	fs.StringVar(&f.Assignee, "assignee", "", "assignee")
	if err := parse(fs, args); err != nil {
		return err
	}
	issues, err := c.svc.ListIssues(ctx, f)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tASSIGNEE\tTITLE")
	for _, i := range issues {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", i.ID, i.Status, i.Assignee, i.Title)
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
	var title, desc, assignee, status string
	fs := c.flagSet("issue update")
	fs.StringVar(&title, "title", "", "title")
	fs.StringVar(&desc, "desc", "", "description")
	fs.StringVar(&assignee, "assignee", "", "assignee")
	fs.StringVar(&status, "status", "", "status")
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
		case "status":
			s := core.Status(status)
			patch.Status = &s
		}
	})
	i, err := c.svc.UpdateIssue(ctx, args[0], patch)
	if err != nil {
		return err
	}
	c.printIssue(i)
	return nil
}

func (c *command) printIssue(i core.Issue) {
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID\t%s\n", i.ID)
	fmt.Fprintf(tw, "Title\t%s\n", i.Title)
	fmt.Fprintf(tw, "Status\t%s\n", i.Status)
	fmt.Fprintf(tw, "Assignee\t%s\n", i.Assignee)
	fmt.Fprintf(tw, "Created\t%s\n", i.CreatedAt.Format("2006-01-02 15:04:05Z07:00"))
	fmt.Fprintf(tw, "Updated\t%s\n", i.UpdatedAt.Format("2006-01-02 15:04:05Z07:00"))
	tw.Flush()
	if i.Description != "" {
		fmt.Fprintf(c.out, "\n%s\n", i.Description)
	}
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
