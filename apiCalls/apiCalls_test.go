package apiCalls

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testPayload = `{
	"Total": 2, "Pages": 1,
	"Items": [
		{"UniqueItemId":1,"ItemId":1,"ApiId":"chaos","Text":"Chaos Orb","CurrentPrice":44.5,"CurrentQuantity":100,
		 "PriceLogs":[{"Price":44.5,"Time":"2026-08-05T00:00:00Z","Quantity":100},{"Price":45.1,"Time":"2026-08-04T00:00:00Z","Quantity":90}]},
		{"UniqueItemId":2,"ItemId":2,"ApiId":"divine","Text":"Divine Orb","CurrentPrice":367.5,"CurrentQuantity":200}
	]
}`

func mockServer(payload string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(payload))
	}))
}

func TestCallApi(t *testing.T) {
	srv := mockServer(testPayload, http.StatusOK)
	defer srv.Close()

	BaseURL = srv.URL + "/poe2/Leagues"
	defer func() { BaseURL = "https://api.poe2scout.com/poe2/Leagues" }()

	data, err := CallApi(http.DefaultClient, "runes", 1)
	if err != nil {
		t.Fatalf("CallApi: %v", err)
	}
	items := data.Items
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Name != "chaos" || items[0].CurrentPrice != 44.5 || items[0].Text != "Chaos Orb" {
		t.Errorf("unexpected first item: %+v", items[0])
	}
	if items[1].Name != "divine" || items[1].CurrentQuantity != 200 || items[1].Text != "Divine Orb" {
		t.Errorf("unexpected second item: %+v", items[1])
	}
	// PriceLogs (daily history) should be parsed for items that carry it.
	if len(items[0].PriceLogs) != 2 {
		t.Fatalf("want 2 PriceLogs entries, got %d: %+v", len(items[0].PriceLogs), items[0].PriceLogs)
	}
	if items[0].PriceLogs[0].Price != 44.5 || items[0].PriceLogs[1].Quantity != 90 {
		t.Errorf("unexpected PriceLogs contents: %+v", items[0].PriceLogs)
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
