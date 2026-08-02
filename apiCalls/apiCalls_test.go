package apiCalls

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testPayload = `{
	"Total": 2, "Pages": 1,
	"Items": [
		{"UniqueItemId":1,"ItemId":1,"ApiId":"chaos","CurrentPrice":44.5,"CurrentQuantity":100},
		{"UniqueItemId":2,"ItemId":2,"ApiId":"divine","CurrentPrice":367.5,"CurrentQuantity":200}
	]
}`

func mockServer(payload string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(payload))
	}))
}

func TestFetchAll(t *testing.T) {
	srv := mockServer(testPayload, http.StatusOK)
	defer srv.Close()

	BaseURL = srv.URL + "/poe2/Leagues"
	defer func() { BaseURL = "https://api.poe2scout.com/poe2/Leagues" }()

	items, err := FetchAll(http.DefaultClient, "runes")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Name != "chaos" || items[0].CurrentPrice != 44.5 {
		t.Errorf("unexpected first item: %+v", items[0])
	}
	if items[1].Name != "divine" || items[1].CurrentQuantity != 200 {
		t.Errorf("unexpected second item: %+v", items[1])
	}
}

func TestCallApiNon200(t *testing.T) {
	srv := mockServer(`oops`, http.StatusInternalServerError)
	defer srv.Close()

	BaseURL = srv.URL + "/poe2/Leagues"
	defer func() { BaseURL = "https://api.poe2scout.com/poe2/Leagues" }()

	if _, err := CallApi(http.DefaultClient, "runes", 1); err == nil {
		t.Fatal("want error for non-200 status, got nil")
	}
}
