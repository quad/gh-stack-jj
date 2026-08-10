package stack

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestRemoteHeadsFiltersOtherRemotes(t *testing.T) {
	got := remoteHeads([]jjBookmark{
		{Name: "main", Remote: "git"},
		{Name: "main", Remote: "origin"},
		{Name: "layer", Remote: "git"},
		{Name: "layer", Remote: "origin"},
		{Name: "layer", Remote: "upstream"},
	}, "origin")
	if len(got) != 2 || got[0] != "layer" || got[1] != "main" {
		t.Fatalf("remoteHeads() = %#v, want [layer main]", got)
	}
}

func TestSyncStackExtendsMatchingPrefix(t *testing.T) {
	runner := &fakeRunner{run: func(name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if name != "gh" || !strings.Contains(joined, "api") {
			t.Fatalf("unexpected command: %s %s", name, joined)
		}
		switch {
		case strings.Contains(joined, "/pulls/11"):
			return `{"number":7,"size":1,"position":1}`, nil
		case strings.Contains(joined, "/pulls/12"), strings.Contains(joined, "/pulls/13"):
			return "null", nil
		case strings.Contains(joined, "/stacks/7") && !strings.Contains(joined, "--method POST"):
			return `{"number":7,"pull_requests":[{"number":11,"state":"open","merged_at":null}]}`, nil
		case strings.Contains(joined, "--method POST"):
			return `{"number":7}`, nil
		default:
			t.Fatalf("unexpected API call: %s", joined)
			return "", nil
		}
	}}

	output := new(strings.Builder)
	if _, err := (Workflow{Runner: runner, Remote: "origin", Repo: "example/repo", Output: output}).syncStackFound(context.Background(), []int{11, 12, 13}); err != nil {
		t.Fatal(err)
	}
	joined := callsText(runner.calls)
	if !strings.Contains(joined, "gh api repos/example/repo/stacks/7/add --method POST --field pull_requests[]=12 --field pull_requests[]=13") {
		t.Fatalf("Stack was not extended:\n%s", joined)
	}
	if got, want := output.String(), "Stack #7 extended with:\n\tPR #12 https://github.com/example/repo/pull/12\n\tPR #13 https://github.com/example/repo/pull/13\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestSyncStackIgnoresMergedLowerPR(t *testing.T) {
	runner := &fakeRunner{run: func(name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if name != "gh" || !strings.Contains(joined, "api") {
			t.Fatalf("unexpected command: %s %s", name, joined)
		}
		switch {
		case strings.Contains(joined, "/pulls/13"):
			return `{"number":7,"size":3,"position":3}`, nil
		case strings.Contains(joined, "/pulls/12"):
			return `{"number":7,"size":3,"position":2}`, nil
		case strings.Contains(joined, "/stacks/7"):
			return `{"number":7,"pull_requests":[{"number":11,"state":"closed","merged_at":"2026-08-10T00:00:00Z"},{"number":12,"state":"open","merged_at":null},{"number":13,"state":"open","merged_at":null}]}`, nil
		default:
			t.Fatalf("unexpected API call: %s", joined)
			return "", nil
		}
	}}

	output := new(strings.Builder)
	if _, err := (Workflow{Runner: runner, Remote: "origin", Repo: "example/repo", Output: output}).syncStackFound(context.Background(), []int{12, 13}); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "" {
		t.Fatalf("output = %q, want no-op", got)
	}
	joined := callsText(runner.calls)
	if strings.Contains(joined, "--method POST") {
		t.Fatalf("no-op attempted to modify Stack:\n%s", joined)
	}
}

func TestSyncStackRestructuresForMissingLowerPR(t *testing.T) {
	runner := &fakeRunner{run: func(name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if name != "gh" || !strings.Contains(joined, "api") {
			t.Fatalf("unexpected command: %s %s", name, joined)
		}
		if strings.Contains(joined, "/pulls/11") {
			return `{"number":7,"size":1,"position":1}`, nil
		}
		if strings.Contains(joined, "/stacks/7") && !strings.Contains(joined, "/unstack") {
			return `{"number":7,"pull_requests":[{"number":11,"state":"open","merged_at":null}]}`, nil
		}
		return `{}`, nil
	}}

	output := new(strings.Builder)
	_, err := (Workflow{Runner: runner, Remote: "origin", Repo: "example/repo", Output: output}).syncStackFound(context.Background(), []int{99, 11})
	if err != nil {
		t.Fatalf("syncStackFound() error = %v, want restructure", err)
	}
	joined := callsText(runner.calls)
	if !strings.Contains(joined, "gh api repos/example/repo/stacks/7/unstack --method POST") {
		t.Fatalf("existing stack was not restructured:\n%s", joined)
	}
	if got := output.String(); got != "" {
		t.Fatalf("output = %q, want empty output", got)
	}
}

func TestPullRequestStackTreatsEmptyResponseAsNoStack(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) { return "", nil }}
	stack, err := (Workflow{Runner: runner, Repo: "example/repo"}).pullRequestStack(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
	if stack != nil {
		t.Fatalf("pullRequestStack() = %#v, want nil", stack)
	}
}

func TestGHAPIArgsUsesEnterpriseHost(t *testing.T) {
	got := (Workflow{Repo: "ghe.example/acme/project"}).ghAPIArgs("repos/acme/project/stacks", "--paginate")
	want := []string{"api", "--hostname", "ghe.example", "repos/acme/project/stacks", "--paginate"}
	if !slices.Equal(got, want) {
		t.Fatalf("ghAPIArgs() = %#v, want %#v", got, want)
	}
}

func TestFindPRRetainsMergedHistory(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) {
		return "[{\"number\":7,\"baseRefName\":\"main\",\"headRefName\":\"layer\",\"state\":\"CLOSED\",\"mergedAt\":\"2026-08-01T00:00:00Z\"}]", nil
	}}
	pr, err := (Workflow{Runner: runner}).findPR(context.Background(), "layer")
	if err != nil {
		t.Fatal(err)
	}
	if pr == nil || pr.Number != 7 {
		t.Fatalf("findPR() = %#v, want merged PR #7", pr)
	}
}

func TestFindPRRejectsMultipleMatches(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) {
		return `[{"number":1,"baseRefName":"main","headRefName":"layer","state":"OPEN"},{"number":2,"baseRefName":"main","headRefName":"layer","state":"OPEN"}]`, nil
	}}
	_, err := (Workflow{Runner: runner}).findPR(context.Background(), "layer")
	if err == nil || !strings.Contains(err.Error(), "multiple open pull requests") {
		t.Fatalf("findPR() error = %v, want ambiguity error", err)
	}
}
