package stack

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func (w Workflow) currentBookmark(ctx context.Context) (string, error) {
	// heads(...) finds the nearest local bookmark. Reading it from log() avoids
	// duplicate local and remote records, and works after `jj new`.
	template := `json(local_bookmarks)`
	out, err := w.runJJ(ctx, "log", "--no-graph", "--revision", "heads(bookmarks() & ::@)", "--template", template)
	if err != nil {
		return "", fmt.Errorf("find current jj bookmark: %w", err)
	}
	bookmarkLists, err := decodeJSONStream[[]jjBookmark](out)
	if err != nil {
		return "", fmt.Errorf("parse current jj bookmark: %w", err)
	}
	var bookmarks []string
	for _, list := range bookmarkLists {
		for _, bookmark := range list {
			bookmarks = append(bookmarks, bookmark.Name)
		}
	}
	if len(bookmarks) == 0 || bookmarks[0] == "" {
		return "", errors.New("current jj revision is not on a bookmark")
	}
	if len(bookmarks) > 1 {
		return "", fmt.Errorf("current jj revision has multiple bookmarks: %s", strings.Join(bookmarks, ", "))
	}
	return bookmarks[0], nil
}

func (w Workflow) trunkBranch(ctx context.Context) (string, error) {
	template := `json(remote_bookmarks)`
	out, err := w.runJJ(ctx, "log", "--no-graph", "--revision", "trunk()", "--template", template)
	if err != nil {
		return "", err
	}
	bookmarkLists, err := decodeJSONStream[[]jjBookmark](out)
	if err != nil {
		return "", fmt.Errorf("parse jj trunk bookmarks: %w", err)
	}
	if len(bookmarkLists) == 0 {
		return "", nil
	}
	branches := remoteHeads(bookmarkLists[0], w.Remote)
	if len(branches) > 1 {
		return "", fmt.Errorf("jj trunk() has multiple %s bookmarks: %s", w.Remote, strings.Join(branches, ", "))
	}
	if len(branches) == 1 {
		return branches[0], nil
	}
	return "", nil
}

func (w Workflow) remoteRepo(ctx context.Context) (string, error) {
	template := `json(git_web_url(` + strconv.Quote(w.Remote) + `))`
	out, err := w.runJJ(ctx, "log", "--no-graph", "--revision", "@", "--template", template)
	if err != nil {
		return "", err
	}
	remoteURLs, err := decodeJSONStream[string](out)
	if err != nil {
		return "", fmt.Errorf("parse jj remote URL: %w", err)
	}
	if len(remoteURLs) == 0 || remoteURLs[0] == "" {
		return "", errors.New("jj returned no web URL")
	}
	remoteURL := remoteURLs[0]
	parsed, err := url.Parse(remoteURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("unsupported remote URL %q", remoteURL)
	}
	path := strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("remote URL %q does not contain OWNER/REPO", remoteURL)
	}
	if parsed.Host == "github.com" {
		return parts[0] + "/" + parts[1], nil
	}
	return parsed.Host + "/" + parts[0] + "/" + parts[1], nil
}

func (w Workflow) resolve(ctx context.Context, revset string) (string, error) {
	out, err := w.runJJ(ctx, "log", "--no-graph", "--revision", "exactly("+revset+", 1)", "--template", "json(commit_id)")
	if err != nil {
		return "", err
	}
	ids, err := decodeJSONStream[string](out)
	if err != nil {
		return "", err
	}
	if len(ids) != 1 {
		return "", fmt.Errorf("revision %q resolves to %d commits, want exactly one", revset, len(ids))
	}
	return ids[0], nil
}

func (w Workflow) layers(ctx context.Context, base, tip string) ([]layer, error) {
	// Remote bookmarks are the stack metadata. Query only the selected remote
	// above the base; each bookmark names a layer head, not one commit.
	rangeRevset := "commit_id(" + strconv.Quote(base) + ")..commit_id(" + strconv.Quote(tip) + ")"
	if w.Base != "" {
		// For an explicit base, require it to be an ancestor of the tip.
		rangeRevset = "exactly(commit_id(" + strconv.Quote(base) + ") & ::commit_id(" + strconv.Quote(tip) + "), 1)..commit_id(" + strconv.Quote(tip) + ")"
	}
	revset := "remote_bookmarks(remote=exact:" + strconv.Quote(w.Remote) + ") & " + rangeRevset
	layers, err := w.bookmarkLayers(ctx, revset)
	if err != nil {
		return nil, err
	}
	if err := w.validateLinear(ctx, layers); err != nil {
		return nil, fmt.Errorf("pushed bookmarks are not a single linear stack: %w", err)
	}
	return layers, nil
}

func (w Workflow) localOnlyBookmarks(ctx context.Context, base, tip string) ([]string, error) {
	revset := "bookmarks() & commit_id(" + strconv.Quote(base) + ")..commit_id(" + strconv.Quote(tip) + ") ~ remote_bookmarks(remote=exact:" + strconv.Quote(w.Remote) + ")"
	out, err := w.runJJ(ctx, "log", "--no-graph", "--revision", revset, "--template", "json(local_bookmarks)")
	if err != nil {
		return nil, err
	}
	bookmarkLists, err := decodeJSONStream[[]jjBookmark](out)
	if err != nil {
		return nil, fmt.Errorf("parse local bookmark output: %w", err)
	}
	seen := make(map[string]struct{})
	for _, list := range bookmarkLists {
		for _, bookmark := range list {
			if bookmark.Name == "" {
				continue
			}
			seen[bookmark.Name] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(seen)), nil
}

func (w Workflow) bookmarkLayers(ctx context.Context, revset string) ([]layer, error) {
	template := `concat(
	'{ "id": ',
	json(commit_id),
	', "bookmarks": ',
	json(remote_bookmarks),
	', "title": ',
	json(description.first_line()),
	' }'
)`
	out, err := w.runJJ(ctx, "log", "--no-graph", "--reversed", "--revision", revset, "--template", template)
	if err != nil {
		return nil, err
	}
	records, err := decodeJSONStream[jjLayer](out)
	if err != nil {
		return nil, fmt.Errorf("parse jj layer output: %w", err)
	}
	var layers []layer
	for _, record := range records {
		if record.ID == "" {
			return nil, errors.New("jj returned a layer without a commit ID")
		}
		heads := remoteHeads(record.Bookmarks, w.Remote)
		if len(heads) != 1 {
			return nil, fmt.Errorf("commit %s has %d %s remote bookmarks, want one", record.ID, len(heads), w.Remote)
		}
		layers = append(layers, layer{ID: record.ID, Head: heads[0], Title: strings.TrimSpace(record.Title)})
	}
	return layers, nil
}

func (w Workflow) validateLinear(ctx context.Context, layers []layer) error {
	for i := 1; i < len(layers); i++ {
		previous := layers[i-1].ID
		current := layers[i].ID
		_, err := w.resolve(ctx, "commit_id("+strconv.Quote(previous)+") & ::commit_id("+strconv.Quote(current)+")")
		if err != nil {
			return fmt.Errorf("check %s before %s: %w", previous, current, err)
		}
	}
	return nil
}

func remoteHeads(bookmarks []jjBookmark, remote string) []string {
	seen := make(map[string]struct{})
	for _, bookmark := range bookmarks {
		if bookmark.Remote != remote {
			continue
		}
		name := bookmark.Name
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}
	return slices.Sorted(maps.Keys(seen))
}

func (w Workflow) runJJ(ctx context.Context, args ...string) (string, error) {
	fixed := []string{"--ignore-working-copy", "--no-pager", "--color=never", "--quiet"}
	return w.Runner.Run(ctx, "jj", append(fixed, args...)...)
}

func remoteBookmarkRevset(name, remote string) string {
	return "remote_bookmarks(exact:" + strconv.Quote(name) + ", remote=exact:" + strconv.Quote(remote) + ")"
}
