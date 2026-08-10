package stack

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestRunJJUsesDeterministicGlobalFlags(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) { return "ok", nil }}
	got, err := (Workflow{Runner: runner}).runJJ(context.Background(), "log", "--template", "commit_id")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("runJJ() = %q, want ok", got)
	}
	want := []string{"jj", "--ignore-working-copy", "--no-pager", "--color=never", "--quiet", "log", "--template", "commit_id"}
	if !slices.Equal(runner.calls[0], want) {
		t.Fatalf("jj args = %#v, want %#v", runner.calls[0], want)
	}
}

func TestResolveRejectsAmbiguousRevision(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) { return jjRecords("one", "two"), nil }}
	_, err := (Workflow{Runner: runner, Remote: "origin"}).resolve(context.Background(), "ambiguous")
	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Fatalf("resolve() error = %v, want ambiguity error", err)
	}
}

func TestCurrentBookmarkRejectsRepeatedName(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) {
		return jjBookmarkRecords(jjBookmark{Name: "feature"}, jjBookmark{Name: "feature"}), nil
	}}
	_, err := (Workflow{Runner: runner}).currentBookmark(context.Background())
	if err == nil || !strings.Contains(err.Error(), "multiple bookmarks") {
		t.Fatalf("currentBookmark() error = %v, want multiple bookmarks error", err)
	}
}

func TestTrunkBranchUsesSelectedRemote(t *testing.T) {
	runner := &fakeRunner{run: func(name string, args ...string) (string, error) {
		if name != "jj" || !strings.Contains(strings.Join(args, " "), "trunk()") {
			t.Fatalf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
		return jjBookmarkRecords(
			jjBookmark{Name: "main", Remote: "upstream"},
			jjBookmark{Name: "main", Remote: "origin"},
		), nil
	}}
	got, err := (Workflow{Runner: runner, Remote: "origin"}).trunkBranch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "main" {
		t.Fatalf("trunkBranch() = %q, want main", got)
	}
}

func TestRemoteRepoParsesGitHubAndEnterpriseURLs(t *testing.T) {
	for _, test := range []struct {
		name string
		url  string
		want string
	}{
		{name: "github", url: "https://github.com/acme/project.git", want: "acme/project"},
		{name: "enterprise", url: "https://ghe.example/acme/project.git/", want: "ghe.example/acme/project"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{run: func(string, ...string) (string, error) {
				encoded, err := json.Marshal(test.url)
				if err != nil {
					t.Fatal(err)
				}
				return string(encoded), nil
			}}
			got, err := (Workflow{Runner: runner, Remote: "origin"}).remoteRepo(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("remoteRepo() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLayersDecodeEscapedFields(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) {
		return jjLayerRecord("commit", "layer@git layer@origin", "  title\twith tab  "), nil
	}}
	layers, err := (Workflow{Runner: runner, Remote: "origin"}).layers(context.Background(), "base-id", "tip-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 1 || layers[0].Title != "title\twith tab" {
		t.Fatalf("layers = %#v, want escaped title preserved", layers)
	}
}

func TestLayersUseSelectedRemoteAndStaleBaseRange(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) {
		return "", nil
	}}
	_, err := (Workflow{Runner: runner, Remote: "origin"}).layers(context.Background(), "current-base", "tip")
	if err != nil {
		t.Fatal(err)
	}
	call := callText(runner.calls[0])
	for _, want := range []string{
		`remote_bookmarks(remote=exact:"origin")`,
		`commit_id("current-base")..commit_id("tip")`,
		`json(remote_bookmarks)`,
	} {
		if !strings.Contains(call, want) {
			t.Errorf("jj layer query %q does not contain %q", call, want)
		}
	}
}

func TestLayersAssertExplicitBaseIsAncestor(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) {
		return "", nil
	}}
	_, err := (Workflow{Runner: runner, Remote: "origin", Base: "main"}).layers(context.Background(), "base-id", "tip-id")
	if err != nil {
		t.Fatal(err)
	}
	call := callText(runner.calls[0])
	if !strings.Contains(call, `exactly(commit_id("base-id") & ::commit_id("tip-id"), 1)..commit_id("tip-id")`) {
		t.Fatalf("explicit base assertion missing from jj layer query: %s", call)
	}
}

func TestLocalOnlyBookmarks(t *testing.T) {
	runner := &fakeRunner{run: func(string, ...string) (string, error) {
		return jjBookmarkRecords(
			jjBookmark{Name: "local-b"},
			jjBookmark{Name: "local-a"},
		), nil
	}}
	got, err := (Workflow{Runner: runner, Remote: "origin"}).localOnlyBookmarks(context.Background(), "base-id", "tip-id")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"local-a", "local-b"}; !slices.Equal(got, want) {
		t.Fatalf("localOnlyBookmarks() = %#v, want %#v", got, want)
	}
	call := callText(runner.calls[0])
	for _, want := range []string{`bookmarks()`, `commit_id("base-id")..commit_id("tip-id")`, `remote_bookmarks(remote=exact:"origin")`} {
		if !strings.Contains(call, want) {
			t.Errorf("local bookmark query %q does not contain %q", call, want)
		}
	}
}
