package stack

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowUsesPushedBookmarksAndPreservesMultiCommitLayers(t *testing.T) {
	prListCalls := make(map[string]int)
	runner := &fakeRunner{run: func(name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "jj" && strings.Contains(joined, "git_web_url"):
			return "\"https://github.com/example/repo\"", nil
		case name == "jj" && strings.Contains(joined, `heads(bookmarks() & ::@)`):
			return jjBookmarkRecords(jjBookmark{Name: "feature"}), nil
		case name == "jj" && strings.Contains(joined, `remote_bookmarks(exact:"feature"`):
			return jjRecords("tip-sha"), nil
		case name == "jj" && strings.Contains(joined, `bookmarks(exact:"feature"`):
			return jjRecords("tip-sha"), nil
		case name == "jj" && strings.Contains(joined, `remote_bookmarks(exact:"main"`):
			return jjRecords("base-sha"), nil
		case name == "jj" && strings.Contains(joined, "trunk()"):
			return jjBookmarkRecords(jjBookmark{Name: "main", Remote: "origin"}), nil
		case name == "jj" && strings.Contains(joined, "bookmarks() &"):
			return "[]", nil
		case name == "jj" && strings.Contains(joined, `commit_id("one-sha") & ::commit_id("two-sha")`):
			return jjRecords("one-sha"), nil
		case name == "jj" && strings.Contains(joined, "remote_bookmarks"):
			return jjLayerRecord("one-sha", "layer-a@git layer-a@origin", "second commit") +
				jjLayerRecord("two-sha", "layer-b@git layer-b@origin", "fourth commit"), nil
		case name == "gh" && strings.Contains(joined, "repo view"):
			return "main\n", nil
		case name == "gh" && strings.Contains(joined, "pr list"):
			head := "layer-a"
			if strings.Contains(joined, "--head layer-b") {
				head = "layer-b"
			}
			prListCalls[head]++
			switch head {
			case "layer-a":
				if prListCalls[head] != 2 {
					return "[]\n", nil
				}
				return `[{"number":11,"baseRefName":"main","headRefName":"layer-a","state":"OPEN"}]`, nil
			case "layer-b":
				if prListCalls[head] != 2 {
					return "[]\n", nil
				}
				return `[{"number":12,"baseRefName":"layer-a","headRefName":"layer-b","state":"OPEN"}]`, nil
			}
			return "[]\n", nil
		case name == "gh" && strings.Contains(joined, "pr create"):
			return "https://github.com/example/repo/pull/1\n", nil
		case name == "gh" && strings.Contains(joined, "/pulls/"):
			return "null\n", nil
		case name == "gh" && strings.Contains(joined, "api") && strings.Contains(joined, "--method POST"):
			return `{"number":99}`, nil
		case name == "gh" && strings.Contains(joined, "api"):
			return "[]\n", nil
		default:
			return "", nil
		}
	}}

	output := new(strings.Builder)
	workflow := Workflow{Runner: runner, Remote: "origin", Output: output}
	if err := workflow.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	joined := callsText(runner.calls)
	checks := []string{
		"gh pr create --base main --head layer-a --title second commit --body ",
		"gh pr create --base layer-a --head layer-b --title fourth commit --body ",
		"gh api repos/example/repo/stacks --method POST --field pull_requests[]=11 --field pull_requests[]=12",
	}
	for _, check := range checks {
		if !strings.Contains(joined, check) {
			t.Errorf("call list does not contain %q:\n%s", check, joined)
		}
	}
	if strings.Contains(joined, "git push") {
		t.Errorf("workflow synthesized or pushed bookmarks:\n%s", joined)
	}
	if !strings.Contains(output.String(), "Stack #99 created with:\n\tPR #11") {
		t.Errorf("unexpected output: %s", output.String())
	}
}

func TestWorkflowUpdatesExistingPullRequestBase(t *testing.T) {
	runner := &fakeRunner{run: func(name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "jj" && strings.Contains(joined, "git_web_url"):
			return "\"https://github.com/example/repo\"", nil
		case name == "jj" && strings.Contains(joined, `heads(bookmarks() & ::@)`):
			return jjBookmarkRecords(jjBookmark{Name: "feature"}), nil
		case name == "jj" && strings.Contains(joined, `remote_bookmarks(exact:"feature"`):
			return jjRecords("tip-sha"), nil
		case name == "jj" && strings.Contains(joined, `bookmarks(exact:"feature"`):
			return jjRecords("tip-sha"), nil
		case name == "jj" && strings.Contains(joined, `remote_bookmarks(exact:"main"`):
			return jjRecords("base-sha"), nil
		case name == "jj" && strings.Contains(joined, "trunk()"):
			return jjBookmarkRecords(jjBookmark{Name: "main", Remote: "origin"}), nil
		case name == "jj" && strings.Contains(joined, "bookmarks() &"):
			return "[]", nil
		case name == "jj" && strings.Contains(joined, "remote_bookmarks"):
			return jjLayerRecord("one-sha", "layer-a@git layer-a@origin", "first change"), nil
		case name == "gh" && strings.Contains(joined, "repo view"):
			return "main\n", nil
		case name == "gh" && strings.Contains(joined, "pr list"):
			return `[{"number":42,"baseRefName":"wrong-base","headRefName":"layer-a","state":"OPEN","url":"https://example.test/42"}]`, nil
		default:
			return "", nil
		}
	}}

	if err := (Workflow{Runner: runner, Remote: "origin", Output: new(strings.Builder)}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := callsText(runner.calls)
	if !strings.Contains(joined, "gh pr edit 42 --base main") {
		t.Fatalf("existing PR base was not updated:\n%s", joined)
	}
	if strings.Contains(joined, "gh pr create") {
		t.Fatalf("existing PR should not be recreated:\n%s", joined)
	}
}

func TestWorkflowDryRunDoesNotCallGitHubPRs(t *testing.T) {
	runner := &fakeRunner{run: func(name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "jj" && strings.Contains(joined, "git_web_url"):
			return "\"https://github.com/example/repo\"", nil
		case name == "jj" && strings.Contains(joined, `heads(bookmarks() & ::@)`):
			return jjBookmarkRecords(jjBookmark{Name: "feature"}), nil
		case name == "jj" && strings.Contains(joined, `remote_bookmarks(exact:"feature"`):
			return jjRecords("tip-sha"), nil
		case name == "jj" && strings.Contains(joined, `bookmarks(exact:"feature"`):
			return jjRecords("tip-sha"), nil
		case name == "jj" && strings.Contains(joined, `remote_bookmarks(exact:"main"`):
			return jjRecords("base-sha"), nil
		case name == "jj" && strings.Contains(joined, "trunk()"):
			return jjBookmarkRecords(jjBookmark{Name: "main", Remote: "origin"}), nil
		case name == "jj" && strings.Contains(joined, "bookmarks() &"):
			return "[]", nil
		case name == "jj" && strings.Contains(joined, "remote_bookmarks"):
			return jjLayerRecord("one-sha", "layer-a@git layer-a@origin", "first change"), nil
		case name == "gh" && strings.Contains(joined, "repo view"):
			return "main\n", nil
		default:
			return "", nil
		}
	}}

	if err := (Workflow{Runner: runner, Remote: "origin", DryRun: true, Output: new(strings.Builder)}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := callsText(runner.calls)
	if strings.Contains(joined, "pr list") || strings.Contains(joined, "pr create") || strings.Contains(joined, "git push") {
		t.Fatalf("dry run performed a mutation or PR lookup:\n%s", joined)
	}
}

func TestSubmitPullRequestsRejectsMissingDescription(t *testing.T) {
	output := new(strings.Builder)
	_, err := (Workflow{Output: output}).submitPullRequests(
		context.Background(),
		"main",
		[]layer{{Head: "layer"}},
	)
	if err == nil || !strings.Contains(err.Error(), "has no description") {
		t.Fatalf("submitPullRequests() error = %v, want missing description error", err)
	}
}

func TestSubmitPullRequestsRebuildsStackForInsertion(t *testing.T) {
	runner := &fakeRunner{run: func(name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "gh" && strings.Contains(joined, "pr list") && strings.Contains(joined, "--head lower"):
			return `[{"number":10,"baseRefName":"main","headRefName":"lower","state":"OPEN"}]`, nil
		case name == "gh" && strings.Contains(joined, "pr list") && strings.Contains(joined, "--head upper"):
			return `[{"number":11,"baseRefName":"main","headRefName":"upper","state":"OPEN"}]`, nil
		case name == "gh" && strings.Contains(joined, "/pulls/11"):
			return `{"number":7}`, nil
		case name == "gh" && strings.Contains(joined, "/stacks/7") && !strings.Contains(joined, "/unstack"):
			return `{"number":7,"pull_requests":[{"number":11,"state":"open","merged_at":null}]}`, nil
		case name == "gh" && strings.Contains(joined, "/unstack"):
			return `{}`, nil
		default:
			return "", nil
		}
	}}

	output := new(strings.Builder)
	layers := []layer{
		{Head: "lower", Title: "lower change"},
		{Head: "upper", Title: "upper change"},
	}
	if _, err := (Workflow{Runner: runner, Repo: "example/repo", Output: output}).submitPullRequests(context.Background(), "main", layers); err != nil {
		t.Fatal(err)
	}
	joined := callsText(runner.calls)
	for _, want := range []string{
		"gh api repos/example/repo/stacks/7",
		"gh api repos/example/repo/stacks/7/unstack --method POST",
		"gh pr edit 11 --base lower",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("call list does not contain %q:\n%s", want, joined)
		}
	}
	if got, want := output.String(), "upper -> PR #11 https://github.com/example/repo/pull/11 (base changed to lower)\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRestructureStackRefusesUntrackedOpenPR(t *testing.T) {
	runner := &fakeRunner{run: func(name string, args ...string) (string, error) {
		if name != "gh" || !strings.Contains(strings.Join(args, " "), "/stacks/7") {
			t.Fatalf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
		return `{"number":7,"pull_requests":[{"number":11,"state":"open","merged_at":null},{"number":12,"state":"open","merged_at":null}]}`, nil
	}}
	workflow := Workflow{Runner: runner, Repo: "example/repo"}
	stack, err := workflow.readStack(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.restructureStack(context.Background(), stack, []int{11}); err == nil || !strings.Contains(err.Error(), "#12") {
		t.Fatalf("restructureStack() error = %v, want missing PR #12", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("restructureStack() made %d calls, want only the read", len(runner.calls))
	}
}
