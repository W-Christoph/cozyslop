package neko_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cozycast/internal/diag"
	"cozycast/internal/neko"
)

// Errors end up in the diagnostics: they name the route, not a member or
// a file.
func TestErrorsNameTheRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	c, err := neko.NewClient(srv.URL, "admin-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	err = c.DeleteMember(ctx, "c-fUBbBBvPPnEE")
	if err == nil || strings.Contains(err.Error(), "fUBbBBvPPnEE") || !strings.Contains(err.Error(), "DELETE /api/members/{id}: 500") {
		t.Errorf("member: %v", err)
	}
	err = c.DeleteFile(ctx, "Some Show - 03.mkv")
	if err == nil || strings.Contains(err.Error(), "Show") || !strings.Contains(err.Error(), "DELETE /api/filetransfer: 500") {
		t.Errorf("file: %v", err)
	}
	// No answer at all: Go's error has the whole URL, which the
	// diagnostics cut to its host.
	srv.Close()
	if err = c.DeleteMember(ctx, "c-fUBbBBvPPnEE"); err == nil || strings.Contains(diag.Scrub(err.Error()), "fUBbBBvPPnEE") {
		t.Errorf("unreachable: %v", err)
	}
}
