package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEditAssets(t *testing.T) {
	h, _, _ := fixture(t)
	for _, name := range []string{"chartedit/beatfit.js", "chartedit/beatfitworker.js", "chartedit/sync.js", "editor/sync.js"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", "/"+name, nil))
		if r.Code != 200 || !strings.HasPrefix(r.Header().Get("Content-Type"), "text/javascript") || r.Body.Len() == 0 {
			t.Errorf("%s: status %d, content type %q", name, r.Code, r.Header().Get("Content-Type"))
		}
	}
}
