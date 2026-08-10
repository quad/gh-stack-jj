package stack

import (
	"encoding/json"
	"io"
	"strings"
)

func decodeJSONStream[T any](out string) ([]T, error) {
	decoder := json.NewDecoder(strings.NewReader(out))
	var values []T
	for {
		var value T
		if err := decoder.Decode(&value); err == io.EOF {
			return values, nil
		} else if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
}
