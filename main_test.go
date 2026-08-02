package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	fmt.Printf("%-10s %10s %10s %10s\n", "name", "current", "buy", "sell")
	for _, r := range recs {
		fmt.Printf("%-10s %10.2f %10.2f %10.2f\n", r.Name, r.Current, r.BuyTarget, r.SellTarget)
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
		price float64
		qty   int64
	}{
		{"exalted", 100.0, 10000},
		{"divine", 367.0, 1000},
	}
	for i := 0; i <= 72; i++ {
		ts := base.Add(time.Duration(i) * time.Hour)
		for _, c := range currencies {
			price := c.price + float64(i)*0.5
			if i >= 69 {
				price *= 1.05
			}
			if _, err := db.Exec(
				`INSERT INTO price_snapshots (name, price, quantity, ts) VALUES (?, ?, ?, ?)`,
				c.name, price, c.qty+int64(i)*10, ts,
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
			{"UniqueItemId":1,"ItemId":1,"ApiId":"chaos","CurrentPrice":44.5,"CurrentQuantity":100},
			{"UniqueItemId":2,"ItemId":2,"ApiId":"divine","CurrentPrice":367.5,"CurrentQuantity":200}
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
