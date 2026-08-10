package stack

import (
	"context"
	"encoding/json"
	"strings"
)

type fakeRunner struct {
	calls [][]string
	run   func(name string, args ...string) (string, error)
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.run(name, args...)
}

func callText(call []string) string {
	return strings.Join(call, " ")
}

func callsText(calls [][]string) string {
	lines := make([]string, len(calls))
	for i, call := range calls {
		lines[i] = callText(call)
	}
	return strings.Join(lines, "\n")
}

func jjRecords(values ...string) string {
	encoded := make([]string, len(values))
	for i, value := range values {
		encodedValue, err := json.Marshal(value)
		if err != nil {
			panic(err)
		}
		encoded[i] = string(encodedValue)
	}
	return strings.Join(encoded, "")
}

func jjBookmarkRecords(values ...jjBookmark) string {
	encoded, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func jjLayerRecord(id, bookmarks, title string) string {
	refs := make([]jjBookmark, 0, len(strings.Fields(bookmarks)))
	for _, field := range strings.Fields(bookmarks) {
		parts := strings.SplitN(field, "@", 2)
		if len(parts) != 2 {
			panic("invalid test bookmark " + field)
		}
		refs = append(refs, jjBookmark{Name: parts[0], Remote: parts[1]})
	}
	value, err := json.Marshal(struct {
		ID        string       `json:"id"`
		Bookmarks []jjBookmark `json:"bookmarks"`
		Title     string       `json:"title"`
	}{id, refs, title})
	if err != nil {
		panic(err)
	}
	return string(value)
}
