package stack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func (w Workflow) defaultBranch(ctx context.Context) (string, error) {
	args := []string{"repo", "view", "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name"}
	args = addRepo(args, w.Repo)
	out, err := w.Runner.Run(ctx, "gh", args...)
	if err != nil {
		return "", fmt.Errorf("find GitHub default branch: %w", err)
	}
	branch := strings.TrimSpace(out)
	if branch == "" {
		return "", errors.New("GitHub repository has no default branch; pass --base")
	}
	return branch, nil
}

func (w Workflow) findPR(ctx context.Context, head string) (*pullRequest, error) {
	args := []string{"pr", "list", "--state", "all", "--head", head, "--limit", "2",
		"--json", "number,baseRefName,headRefName,state,mergedAt,url"}
	args = addRepo(args, w.Repo)
	out, err := w.Runner.Run(ctx, "gh", args...)
	if err != nil {
		return nil, err
	}
	var prs []pullRequest
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", err)
	}
	for _, pr := range prs {
		if pr.Number <= 0 || pr.HeadRefName != head {
			return nil, fmt.Errorf("GitHub returned invalid pull request for head %q", head)
		}
	}
	if len(prs) == 0 {
		return nil, nil
	}
	var open *pullRequest
	for i := range prs {
		if prs[i].State != "OPEN" {
			continue
		}
		if open != nil {
			return nil, fmt.Errorf("multiple open pull requests found for head %q", head)
		}
		open = &prs[i]
	}
	if open != nil {
		return open, nil
	}
	if len(prs) > 1 {
		return nil, fmt.Errorf("multiple historical pull requests found for head %q", head)
	}
	if prs[0].MergedAt == "" {
		return nil, fmt.Errorf("pull request #%d for head %q is closed without merging; reopen it or remove the bookmark", prs[0].Number, head)
	}
	return &prs[0], nil
}

func (w Workflow) createPR(ctx context.Context, head, base, title string) error {
	args := []string{"pr", "create", "--base", base, "--head", head, "--title", title, "--body", ""}
	if w.Draft {
		args = append(args, "--draft")
	}
	args = addRepo(args, w.Repo)
	_, err := w.Runner.Run(ctx, "gh", args...)
	if err != nil {
		return err
	}
	return nil
}

func (w Workflow) editBase(ctx context.Context, number int, base string) error {
	args := []string{"pr", "edit", fmt.Sprint(number), "--base", base}
	args = addRepo(args, w.Repo)
	_, err := w.Runner.Run(ctx, "gh", args...)
	return err
}

func (w Workflow) verifyBaseBranches(ctx context.Context, changes []baseChange) error {
	_, repo := repoParts(w.Repo)
	seen := make(map[string]bool, len(changes))
	for _, change := range changes {
		if seen[change.Base] {
			continue
		}
		seen[change.Base] = true
		endpoint := "repos/" + repo + "/branches/" + url.PathEscape(change.Base)
		if _, err := w.Runner.Run(ctx, "gh", w.ghAPIArgs(endpoint)...); err != nil {
			return fmt.Errorf("base branch %q is not available on GitHub; push it to %q before stacking: %w", change.Base, w.Remote, err)
		}
	}
	return nil
}

func (w Workflow) syncStackFound(ctx context.Context, prNumbers []int) (bool, error) {
	var matching *stackMembership
	for i := range slices.Backward(prNumbers) {
		stack, err := w.pullRequestStack(ctx, prNumbers[i])
		if err != nil {
			return false, fmt.Errorf("find stack for PR #%d: %w", prNumbers[i], err)
		}
		if stack != nil {
			matching = stack
			break
		}
	}
	if matching == nil {
		return false, nil
	}

	stack, err := w.readStack(ctx, matching.Number)
	if err != nil {
		return false, err
	}
	plan, err := planStack(stack, prNumbers)
	if err != nil {
		return false, err
	}
	switch plan.Action {
	case stackNoop:
		return true, nil
	case stackAppend:
		if err := w.extendStack(ctx, matching.Number, plan.Delta); err != nil {
			return false, err
		}
		fmt.Fprint(w.Output, "Stack #", matching.Number, " extended with:\n")
		w.writePRList(plan.Delta)
		return true, nil
	case stackRestructure:
		if err := w.restructureStack(ctx, stack, prNumbers); err != nil {
			return false, err
		}
		return false, nil
	}
	return false, fmt.Errorf("unknown Stack action %d", plan.Action)
}

func (w Workflow) writePR(number int, url string) {
	if url == "" {
		url = w.pullRequestURL(number)
	}
	fmt.Fprint(w.Output, "PR #", number)
	if url != "" {
		fmt.Fprint(w.Output, " ", url)
	}
}

func (w Workflow) writePRList(numbers []int) {
	for _, number := range numbers {
		fmt.Fprint(w.Output, "\t")
		w.writePR(number, "")
		fmt.Fprintln(w.Output)
	}
}

func (w Workflow) pullRequestURL(number int) string {
	host, path := repoParts(w.Repo)
	if path == "" {
		return ""
	}
	if host == "" {
		host = "github.com"
	}
	return "https://" + host + "/" + path + "/pull/" + strconv.Itoa(number)
}

func (w Workflow) pullRequestStack(ctx context.Context, number int) (*stackMembership, error) {
	endpoint := w.pullRequestsEndpoint() + "/" + strconv.Itoa(number)
	out, err := w.Runner.Run(ctx, "gh", w.ghAPIArgs(endpoint, "--jq", ".stack")...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	var stack *stackMembership
	if err := json.Unmarshal([]byte(out), &stack); err != nil {
		return nil, fmt.Errorf("parse PR #%d stack membership: %w", number, err)
	}
	if stack == nil {
		return nil, nil
	}
	if stack.Number <= 0 {
		return nil, fmt.Errorf("GitHub returned invalid stack membership for PR #%d", number)
	}
	return stack, nil
}

func (w Workflow) readStack(ctx context.Context, number int) (stackResource, error) {
	endpoint := fmt.Sprintf("%s/%d", w.stackEndpoint(), number)
	out, err := w.Runner.Run(ctx, "gh", w.ghAPIArgs(endpoint)...)
	if err != nil {
		return stackResource{}, fmt.Errorf("read Stack #%d: %w", number, err)
	}
	var stack stackResource
	if err := json.Unmarshal([]byte(out), &stack); err != nil {
		return stackResource{}, fmt.Errorf("parse Stack #%d: %w", number, err)
	}
	return stack, nil
}

func (w Workflow) restructureStack(ctx context.Context, stack stackResource, desired []int) error {
	number := stack.Number
	if err := validateStackRebuild(stack, desired); err != nil {
		return err
	}

	unstackEndpoint := fmt.Sprintf("%s/%d/unstack", w.stackEndpoint(), number)
	if _, err := w.Runner.Run(ctx, "gh", w.ghAPIArgs(unstackEndpoint, "--method", "POST")...); err != nil {
		return fmt.Errorf("clear Stack #%d before restructuring: %w", number, err)
	}
	return nil
}

func planStack(stack stackResource, desired []int) (stackPlan, error) {
	active := stackActiveNumbers(stack)
	if isPrefix(desired, active) {
		return stackPlan{Action: stackNoop}, nil
	}
	if isPrefix(active, desired) {
		return stackPlan{Action: stackAppend, Delta: desired[len(active):]}, nil
	}
	if err := validateStackRebuild(stack, desired); err != nil {
		return stackPlan{}, err
	}
	return stackPlan{Action: stackRestructure}, nil
}

func validateStackRebuild(stack stackResource, desired []int) error {
	number := stack.Number
	desiredSet := make(map[int]bool, len(desired))
	for _, pr := range desired {
		desiredSet[pr] = true
	}
	var missing []string
	for _, pr := range stack.PullRequests {
		if desiredSet[pr.Number] {
			if pr.MergedAt != nil {
				return fmt.Errorf("Stack #%d contains merged PR #%d; cannot insert into that Stack", number, pr.Number)
			}
			continue
		}
		if pr.MergedAt == nil {
			missing = append(missing, "#"+strconv.Itoa(pr.Number))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("Stack #%d also contains open PRs %s; cannot restructure it without dropping them", number, strings.Join(missing, ", "))
	}
	return nil
}

func stackActiveNumbers(stack stackResource) []int {
	var numbers []int
	for _, pr := range stack.PullRequests {
		if pr.MergedAt == nil {
			numbers = append(numbers, pr.Number)
		}
	}
	return numbers
}

func isPrefix(prefix, values []int) bool {
	return len(prefix) <= len(values) && slices.Equal(prefix, values[:len(prefix)])
}

func (w Workflow) createStack(ctx context.Context, prNumbers []int) (int, error) {
	args := w.ghAPIArgs(w.stackEndpoint(), "--method", "POST")
	for _, number := range prNumbers {
		args = append(args, "--field", fmt.Sprintf("pull_requests[]=%d", number))
	}
	out, err := w.Runner.Run(ctx, "gh", args...)
	if err != nil {
		return 0, fmt.Errorf("create Stack: %w", err)
	}
	var stack struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(out), &stack); err != nil {
		return 0, fmt.Errorf("parse created stack: %w", err)
	}
	if stack.Number <= 0 {
		return 0, errors.New("stack response did not include a valid number")
	}
	return stack.Number, nil
}

func (w Workflow) extendStack(ctx context.Context, stackNumber int, prNumbers []int) error {
	args := w.ghAPIArgs(fmt.Sprintf("%s/%d/add", w.stackEndpoint(), stackNumber), "--method", "POST")
	for _, number := range prNumbers {
		args = append(args, "--field", fmt.Sprintf("pull_requests[]=%d", number))
	}
	if _, err := w.Runner.Run(ctx, "gh", args...); err != nil {
		return fmt.Errorf("extend Stack #%d: %w", stackNumber, err)
	}
	return nil
}

func (w Workflow) stackEndpoint() string {
	_, repo := repoParts(w.Repo)
	return "repos/" + repo + "/stacks"
}

func (w Workflow) pullRequestsEndpoint() string {
	_, repo := repoParts(w.Repo)
	return "repos/" + repo + "/pulls"
}

func (w Workflow) ghAPIArgs(endpoint string, args ...string) []string {
	host, _ := repoParts(w.Repo)
	apiArgs := []string{"api"}
	if host != "" {
		apiArgs = append(apiArgs, "--hostname", host)
	}
	apiArgs = append(apiArgs, endpoint)
	return append(apiArgs, args...)
}

func repoParts(repo string) (host, path string) {
	parts := strings.Split(strings.Trim(repo, "/"), "/")
	if len(parts) == 3 {
		return parts[0], parts[1] + "/" + parts[2]
	}
	return "", strings.Trim(repo, "/")
}

func addRepo(args []string, repo string) []string {
	return append(args, "--repo", repo)
}
