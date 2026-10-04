package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadJSONRequiresEndOfBody(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"object", `{"name":"alice"}`, 200},
		{"whitespace", "{\"name\":\"alice\"} \n\t", 200},
		{"second object", `{"name":"alice"}{}`, 400},
		{"null", `{"name":"alice"} null`, 400},
		{"array", `{"name":"alice"} []`, 400},
		{"junk", `{"name":"alice"} junk`, 400},
		{"unfinished", `{"name":"alice"} {`, 400},
		{"unknown field", `{"unknown":true}`, 400},
		{"empty", "", 400},
		{"oversized object", `{"name":"` + strings.Repeat("a", maxJSONBody) + `"}`, 413},
		{"oversized trailing whitespace", `{}` + strings.Repeat(" ", maxJSONBody), 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			var out struct{ Name string }
			if got := readJSON(w, r, &out); got != (tt.status == 200) || w.Code != tt.status {
				t.Fatalf("readJSON=%v status=%d body=%s", got, w.Code, w.Body)
			}
			if tt.status >= 400 {
				var failure struct{ Error string }
				if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil || failure.Error == "" {
					t.Fatalf("missing JSON error: %s (%v)", w.Body, err)
				}
			}
		})
	}
}
