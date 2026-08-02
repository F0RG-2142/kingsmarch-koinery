package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kingsmarch-koinery/apiCalls"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := ensureSchema(db); err != nil {
		t.Fatal(err)
	}
	return db
}

// buildEmbedPreview renders a sample embed so you can preview what Discord will show.
func TestBuildEmbedPreview(t *testing.T) {
	recs := []recommendation{
		{Name: "divine", Display: "Divine Orb", Current: 423.15, BuyTarget: 376.47, SellTarget: 422.94},
		{Name: "exalted", Display: "Exalted Orb", Current: 142.80, BuyTarget: 119.21, SellTarget: 143.03},
		{Name: "chaos", Display: "Chaos Orb", Current: 44.50, BuyTarget: 40.00, SellTarget: 48.00},
		{Name: "mirror", Display: "Mirror of Kalandra", Current: 1767897.60, BuyTarget: 1600000.00, SellTarget: 1800000.00},
	}
	payload := buildEmbed(recs, time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC))

	// Preview the exact JSON that would be POSTed to the webhook.
	fmt.Printf("\n--- discord embed payload ---\n%s\n", string(payload))

	var p discordPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("embed is not valid JSON: %v", err)
	}
	if len(p.Embeds) != 1 {
		t.Fatalf("want 1 embed, got %d", len(p.Embeds))
	}
	if p.Embeds[0].Description == "" {
		t.Errorf("embed should have a description line")
	}
	fields := p.Embeds[0].Fields
	if len(fields) != 4 {
		t.Fatalf("want 4 fields (one per currency), got %d", len(fields))
	}
	for _, f := range fields {
		if f.Inline {
			t.Errorf("field %q should be full-width", f.Name)
		}
		for _, want := range []string{"```", "Current", "Buy", "Sell"} {
			if !strings.Contains(f.Value, want) {
				t.Errorf("field %q value missing %q: %q", f.Name, want, f.Value)
			}
		}
	}
}

func TestColdStartGuard(t *testing.T) {
	db := openTestDB(t)
	if _, err := analyze(db, time.Now()); err != errNotEnoughHistory {
		t.Fatalf("want errNotEnoughHistory, got %v", err)
	}
}

func TestAnalyze(t *testing.T) {
	db := openTestDB(t)
	seed(t, db)

	recs, err := analyze(db, time.Now())
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 recommendations, got %d", len(recs))
	}
	// Preview what the analysis produces (Discord formatting comes later).
	fmt.Printf("\n--- analysis preview ---\n")
	fmt.Printf("%-16s %10s %10s %10s\n", "name", "current", "buy", "sell")
	for _, r := range recs {
		fmt.Printf("%-16s %10.2f %10.2f %10.2f\n", r.Display, r.Current, r.BuyTarget, r.SellTarget)
		if r.BuyTarget >= r.SellTarget {
			t.Errorf("%s: buy %.2f should be below sell %.2f", r.Name, r.BuyTarget, r.SellTarget)
		}
		if r.Current <= 0 {
			t.Errorf("%s: current price %.2f should be positive", r.Name, r.Current)
		}
	}
}

// seed writes 72h of synthetic snapshots for two currencies, with a short-term
// price surge in the final 3h so the VMS skew is exercised.
func seed(t *testing.T, db *sql.DB) {
	t.Helper()
	now := time.Now().Truncate(time.Hour)
	base := now.Add(-72 * time.Hour)
	currencies := []struct {
		name  string
		text  string
		price float64
		qty   int64
	}{
		{"exalted", "Exalted Orb", 100.0, 10000},
		{"divine", "Divine Orb", 367.0, 1000},
	}
	for i := 0; i <= 72; i++ {
		ts := base.Add(time.Duration(i) * time.Hour)
		for _, c := range currencies {
			price := c.price + float64(i)*0.5
			if i >= 69 {
				price *= 1.05
			}
			if _, err := db.Exec(
				`INSERT INTO price_snapshots (name, price, quantity, ts, text) VALUES (?, ?, ?, ?, ?)`,
				c.name, price, c.qty+int64(i)*10, ts, c.text,
			); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestStoreSnapshot(t *testing.T) {
	payload := `{
		"Total": 2, "Pages": 1,
		"Items": [
			{"UniqueItemId":1,"ItemId":1,"ApiId":"chaos","Text":"Chaos Orb","CurrentPrice":44.5,"CurrentQuantity":100},
			{"UniqueItemId":2,"ItemId":2,"ApiId":"divine","Text":"Divine Orb","CurrentPrice":367.5,"CurrentQuantity":200}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	old := apiCalls.BaseURL
	apiCalls.BaseURL = srv.URL + "/poe2/Leagues"
	defer func() { apiCalls.BaseURL = old }()

	db := openTestDB(t)
	if err := storeSnapshot(db, http.DefaultClient, "runes"); err != nil {
		t.Fatalf("storeSnapshot: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM price_snapshots`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 rows stored, got %d", n)
	}
}
