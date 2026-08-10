package stack

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func (w Workflow) Run(ctx context.Context) error {
	if w.Repo == "" {
		repo, err := w.remoteRepo(ctx)
		if err != nil {
			return fmt.Errorf("derive GitHub repository from jj remote %q: %w (pass --repo)", w.Remote, err)
		}
		w.Repo = repo
	}
	branch := w.Bookmark
	var err error
	if branch == "" {
		branch, err = w.currentBookmark(ctx)
		if err != nil {
			return err
		}
	}
	tip, err := w.resolve(ctx, remoteBookmarkRevset(branch, w.Remote))
	if err != nil {
		return fmt.Errorf("current bookmark %q has not been pushed to %q: %w", branch, w.Remote, err)
	}
	localTip, err := w.resolve(ctx, "bookmarks(exact:"+strconv.Quote(branch)+")")
	if err != nil {
		return fmt.Errorf("resolve local bookmark %q: %w", branch, err)
	}
	if localTip != tip {
		return fmt.Errorf("bookmark %q differs from %s; push/update it before stacking", branch, w.Remote)
	}

	base, err := w.baseBranch(ctx)
	if err != nil {
		return err
	}
	baseRev, err := w.resolve(ctx, "coalesce("+remoteBookmarkRevset(base, w.Remote)+", bookmarks(exact:"+strconv.Quote(base)+"))")
	if err != nil {
		return fmt.Errorf("cannot resolve base bookmark %q on %q or locally: %w", base, w.Remote, err)
	}
	localOnly, err := w.localOnlyBookmarks(ctx, baseRev, tip)
	if err != nil {
		return fmt.Errorf("find local bookmarks not pushed to %q: %w", w.Remote, err)
	}
	if len(localOnly) > 0 {
		fmt.Fprintln(w.Output, "local bookmarks not pushed to", w.Remote+":", strings.Join(localOnly, ", "))
	}
	layers, err := w.layers(ctx, baseRev, tip)
	if err != nil {
		return fmt.Errorf("find pushed bookmarks between %s and %s: %w", base, branch, err)
	}
	if len(layers) == 0 {
		return nil
	}
	activePRs, err := w.submitPullRequests(ctx, base, layers)
	if err != nil {
		return err
	}
	// Check existing Stacks before the fewer-than-two shortcut.
	found, err := w.syncStackFound(ctx, activePRs)
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	if len(activePRs) < 2 {
		return nil
	}
	created, err := w.createStack(ctx, activePRs)
	if err != nil {
		return err
	}
	fmt.Fprint(w.Output, "Stack #", created, " created with:\n")
	w.writePRList(activePRs)
	return nil
}

func (w Workflow) baseBranch(ctx context.Context) (string, error) {
	if w.Base != "" {
		return w.Base, nil
	}
	base, err := w.trunkBranch(ctx)
	if err != nil {
		return "", fmt.Errorf("find jj trunk: %w", err)
	}
	if base != "" {
		return base, nil
	}
	return w.defaultBranch(ctx)
}

func (w Workflow) submitPullRequests(ctx context.Context, base string, layers []layer) ([]int, error) {
	parent := base
	var submitted []int
	var baseChanges []baseChange
	for _, layer := range layers {
		if layer.Title == "" {
			return nil, fmt.Errorf("bookmark %q has no description", layer.Head)
		}
		head := layer.Head
		if w.DryRun {
			fmt.Fprint(w.Output, "\t", head, " -> ", layer.Title, " (base ", parent, ")\n")
			parent = head
			continue
		}

		created := false
		pr, err := w.findPR(ctx, head)
		if err != nil {
			return nil, fmt.Errorf("find pull request for %s: %w", head, err)
		}
		if pr == nil {
			err = w.createPR(ctx, head, parent, layer.Title)
			if err != nil {
				return nil, fmt.Errorf("create pull request for %s: %w", head, err)
			}
			created = true
			pr, err = w.findPR(ctx, head)
			if err != nil || pr == nil {
				if err == nil {
					err = errors.New("created pull request was not returned by GitHub")
				}
				return nil, fmt.Errorf("read created pull request for %s: %w", head, err)
			}
		}

		if created {
			fmt.Fprint(w.Output, head, " -> ")
			w.writePR(pr.Number, pr.URL)
			fmt.Fprint(w.Output, " (base ", parent, ")")
			fmt.Fprintln(w.Output)
		} else if pr.State == "OPEN" && pr.BaseRefName != parent {
			baseChanges = append(baseChanges, baseChange{
				Head:   head,
				Number: pr.Number,
				URL:    pr.URL,
				Base:   parent,
			})
		}
		submitted = append(submitted, pr.Number)
		parent = head
	}

	if len(baseChanges) == 0 {
		return submitted, nil
	}

	desired := submitted
	if err := w.verifyBaseBranches(ctx, baseChanges); err != nil {
		return nil, err
	}
	restructured := make(map[int]bool)
	for _, change := range baseChanges {
		stack, err := w.pullRequestStack(ctx, change.Number)
		if err != nil {
			return nil, fmt.Errorf("find Stack for PR #%d: %w", change.Number, err)
		}
		if stack == nil || restructured[stack.Number] {
			continue
		}
		stackData, err := w.readStack(ctx, stack.Number)
		if err != nil {
			return nil, err
		}
		if err := w.restructureStack(ctx, stackData, desired); err != nil {
			return nil, err
		}
		restructured[stack.Number] = true
	}
	for _, change := range baseChanges {
		if err := w.editBase(ctx, change.Number, change.Base); err != nil {
			return nil, fmt.Errorf("set base of pull request #%d to %s: %w", change.Number, change.Base, err)
		}
		fmt.Fprint(w.Output, change.Head, " -> ")
		w.writePR(change.Number, change.URL)
		fmt.Fprint(w.Output, " (base changed to ", change.Base, ")\n")
	}
	return submitted, nil
}
