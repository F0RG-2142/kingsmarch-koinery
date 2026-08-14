package main

import (
	"database/sql"
	"fmt"
	"image/png"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"kingsmarch-koinery/apiCalls"
	"kingsmarch-koinery/render"
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
	fmt.Printf("%-16s %10s %10s %10s\n", "name", "current", "buy", "sell")
	for _, r := range recs {
		fmt.Printf("%-16s %10.2f %10.2f %10.2f\n", r.Display, r.Current, r.BuyTarget, r.SellTarget)
		if r.BuyTarget >= r.SellTarget {
			t.Errorf("%s: buy %.2f should be below sell %.2f", r.Name, r.BuyTarget, r.SellTarget)
		}
		if r.Current <= 0 {
			t.Errorf("%s: current price %.2f should be positive", r.Name, r.Current)
		}
		if r.HalfLife <= 0 {
			t.Errorf("%s: expected a positive mean-reversion half-life, got %v", r.Name, r.HalfLife)
		}
	}
}

// TestHalfLife verifies the OU mean-reversion filter: an oscillating series is
// flagged as reverting (ok=true) while a trending series is rejected.
func TestHalfLife(t *testing.T) {
	var reverting []float64
	for i := 0; i < 100; i++ {
		reverting = append(reverting, 100+math.Sin(float64(i)/4.0)*10)
	}
	hl, ok := ouHalfLife(reverting, time.Hour)
	if !ok || hl <= 0 {
		t.Fatalf("oscillating series should mean-revert, got ok=%v hl=%v", ok, hl)
	}

	var trending []float64
	for i := 0; i < 100; i++ {
		trending = append(trending, float64(i)*0.5)
	}
	if _, ok := ouHalfLife(trending, time.Hour); ok {
		t.Fatalf("trending series should not be flagged as mean-reverting")
	}
}

// TestBacktestSim verifies the band strategy simulation and grid search produce
// trades on an oscillating (mean-reverting) price series.
func TestBacktestSim(t *testing.T) {
	// Sine oscillation with occasional large excursions so the bands are hit.
	prices := make([]float64, 0, 300)
	for i := 0; i < 300; i++ {
		p := 100 + math.Sin(float64(i)/6.0)*20
		switch i % 25 {
		case 0:
			p = 40 // deep dip
		case 12:
			p = 170 // big spike
		}
		prices = append(prices, p)
	}

	rets, _ := bandSim(prices, backtestWindow, 2.0, 1.5)
	if len(rets) == 0 {
		t.Fatalf("bandSim produced no trades on an oscillating series")
	}

	best := bestParams(prices, backtestWindow)
	if best.Trades == 0 {
		t.Fatalf("bestParams found no tradeable configuration")
	}
	if best.WinRate < 0 || best.WinRate > 1 {
		t.Fatalf("win rate out of range: %v", best.WinRate)
	}
}

// seed writes 72h of synthetic snapshots for two currencies, with a short-term
// price surge in the final 3h so the VMS skew is exercised. Each price is an
// AR(1) mean-reverting process (strong reversion, φ=0.8) so the OU half-life
// filter lets it through.
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
	phi := 0.8
	for _, c := range currencies {
		rng := rand.New(rand.NewSource(1))
		price := c.price
		for i := 0; i <= 72; i++ {
			price = c.price + phi*(price-c.price) + rng.NormFloat64()*0.01*c.price
			p := price
			if i >= 69 {
				p *= 1.05 // short-term surge → mu_short > mu
			}
			if _, err := db.Exec(
				`INSERT INTO price_snapshots (name, price, quantity, ts, text) VALUES (?, ?, ?, ?, ?)`,
				c.name, p, c.qty+int64(i)*10, base.Add(time.Duration(i)*time.Hour), c.text,
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

// TestRenderTable renders a sample table to PNG and writes it to a temp file
// so a human can open it for visual inspection. Also verifies the image
// height grows with the number of rows.
func TestRenderTable(t *testing.T) {
	recs := []render.Recommendation{
		{Name: "divine", Display: "Divine Orb", Current: 423.15, BuyTarget: 376.47, SellTarget: 422.94, BulkSell: 422.94},
		{Name: "exalted", Display: "Exalted Orb", Current: 142.80, BuyTarget: 119.21, SellTarget: 143.03, BulkSell: 143.03},
		{Name: "chaos", Display: "Chaos Orb", Current: 44.50, BuyTarget: 40.00, SellTarget: 48.00, BulkSell: 51.00},
		{Name: "mirror", Display: "Mirror of Kalandra", Current: 1767897.60, BuyTarget: 1600000.00, SellTarget: 1800000.00, BulkSell: 1800000.00},
	}
	img4 := render.Table(recs, "runes", "PoE2 Currency Analysis")
	b4 := img4.Bounds()
	if b4.Dx() < 1200 {
		t.Errorf("image should be at least 1200px wide, got %d", b4.Dx())
	}
	if b4.Dy() <= 0 {
		t.Fatalf("image has zero height: %v", b4)
	}

	// Render with more rows and confirm it gets taller.
	recsBig := make([]render.Recommendation, 0, 10)
	for i := 0; i < 10; i++ {
		recsBig = append(recsBig, recs[i%len(recs)])
	}
	img10 := render.Table(recsBig, "runes", "PoE2 Currency Analysis")
	b10 := img10.Bounds()
	if b10.Dy() <= b4.Dy() {
		t.Errorf("10-row image (%d) should be taller than 4-row (%d)", b10.Dy(), b4.Dy())
	}
	if b10.Dx() != b4.Dx() {
		t.Errorf("width should be the same regardless of row count: 4-row=%d 10-row=%d", b4.Dx(), b10.Dx())
	}

	out, err := os.CreateTemp("", "test-render-*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(out.Name())
	if err := png.Encode(out, img4); err != nil {
		t.Fatal(err)
	}
	out.Close()
	fmt.Printf("\n--- rendered PNG: %s (%dx%d, 10-row would be %dx%d) ---\n",
		out.Name(), b4.Dx(), b4.Dy(), b10.Dx(), b10.Dy())

	// Round-trip decode to confirm it's a valid PNG.
	f, err := os.Open(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoded, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != b4 {
		t.Errorf("decoded bounds %v != original %v", decoded.Bounds(), b4)
	}
}

// TestAnalyzeFiltersNoise verifies that a currency with only one snapshot
// (and no volume) is filtered out by the safeguards.
func TestAnalyzeFiltersNoise(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().Truncate(time.Hour)
	// Need at least 48h of history to pass the cold-start guard.
	oldTs := now.Add(-72 * time.Hour)
	// Single snapshot, tiny volume — should be filtered out.
	if _, err := db.Exec(
		`INSERT INTO price_snapshots (name, price, quantity, ts, text) VALUES (?, ?, ?, ?, ?)`,
		"artificers", 3.22, 1, oldTs, "Artificer's Orb",
	); err != nil {
		t.Fatal(err)
	}
	// Five snapshots at constant price — still filtered: total_volume=5 < 50.
	base := now.Add(-30 * time.Hour)
	for i := 0; i < 5; i++ {
		if _, err := db.Exec(
			`INSERT INTO price_snapshots (name, price, quantity, ts, text) VALUES (?, ?, ?, ?, ?)`,
			"decent", 10.0, 1, base.Add(time.Duration(i)*time.Hour), "Decent",
		); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := analyze(db, now)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected 0 recommendations after noise filtering, got %d: %v", len(recs), recs)
	}
}

// TestSendMultipart assembles a multipart POST and verifies the structure
// (a file part and a payload_json field with a valid embed).
func TestSendMultipart(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("want multipart, got %q", ct)
		}
		// Read the entire body so we can inspect the trailing payload_json.
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	recs := []recommendation{
		{Name: "divine", Display: "Divine Orb", Current: 423.15, BuyTarget: 376.47, SellTarget: 422.94, BulkSell: 422.94},
	}
	if err := sendAnalysisDiscord([]string{srv.URL}, recs, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(received), "payload_json") {
		t.Errorf("body missing payload_json field")
	}
	if !strings.Contains(string(received), "attachment://") {
		t.Errorf("body missing attachment reference")
	}
}

