package model

import (
	"encoding/json"
	"testing"
)

func TestStudentsKopiadataShapes(t *testing.T) {
	for _, in := range []string{`{"kopiadata":"x"}`, `{"kopiadata":[]}`, `{"kopiadata":[{"a":1}]}`, `{}`} {
		var s Students
		if err := json.Unmarshal([]byte(in), &s); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
}
