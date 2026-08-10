package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/quad/gh-stack-jj/internal/stack"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("gh-stack-jj", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	remote := flags.String("remote", "origin", "jj Git remote containing the pushed branch")
	bookmark := flags.String("bookmark", "", "jj bookmark to stack (defaults to the nearest bookmark at @)")
	base := flags.String("base", "", "base branch (defaults to jj trunk or the repository's GitHub default branch)")
	repo := flags.String("repo", "", "GitHub repository in OWNER/REPO form")
	draft := flags.Bool("draft", false, "create new pull requests as drafts")
	dryRun := flags.Bool("dry-run", false, "show the stack without pushing or changing pull requests")
	debug := flags.Bool("debug", false, "log jj and GitHub commands with timings")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	debugOutput := io.Discard
	if *debug {
		debugOutput = os.Stderr
	}
	runner := stack.NewExecRunner(debugOutput)
	workflow := stack.Workflow{
		Runner:   runner,
		Remote:   *remote,
		Bookmark: *bookmark,
		Base:     *base,
		Repo:     *repo,
		Draft:    *draft,
		DryRun:   *dryRun,
		Output:   os.Stdout,
	}
	return workflow.Run(ctx)
}
