package gmtool

import (
	"database/sql"
	"net/http/httptest"
	"testing"

	"dnf90.local/gateway/internal/modules/dnf/logincodes"
)

func TestPublicGiftRejectsMissingServiceAndMalformedBearer(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.PublicGiftHandler().ServeHTTP(w, httptest.NewRequest("POST", "/api/gifts/redeem", nil))
	if w.Code != 503 {
		t.Fatalf("missing store: %d", w.Code)
	}
	db, err := sql.Open("mysql", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.deps.LoginCodes, err = logincodes.NewStore(db, "accounts")
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"", "Basic abc", "Bearer short", "de509f65000000000000000000000000"} {
		r := httptest.NewRequest("GET", "/api/gifts/characters", nil)
		r.Header.Set("Authorization", header)
		w = httptest.NewRecorder()
		s.PublicGiftHandler().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("header %q: %d", header, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("authenticated response may be cached")
		}
	}
}
