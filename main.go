package main

import (
	"database/sql"
	"flag"
	"fmt"
	"kingsmarch-koinery/apiCalls"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env if present (missing file is fine).
	_ = godotenv.Load()

	printMode := flag.Bool("print", false, "print all stored snapshots and exit")
	wipe := flag.Bool("wipe", false, "delete the storage file and exit")
	analyzeMode := flag.Bool("analyze", false, "run analysis once and exit")
	backtestMode := flag.Bool("backtest", false, "backtest band multipliers on PriceLogs and print the best (k,j) per currency, then exit")
	sendMode := flag.Bool("send", false, "run analysis once and send to Discord, then exit")
	testSendMode := flag.Bool("send-test", false, "send a sample embed to Discord to verify the webhook, then exit")
	flag.Parse()

	client := http.DefaultClient

	league := os.Getenv("LEAGUE")
	if league == "" {
		league = "runes"
	}

	// Persistent DuckDB file so hourly snapshots survive restarts.
	dbPath := os.Getenv("DUCKDB_PATH")
	if dbPath == "" {
		dbPath = "./data/prices.duckdb"
	}

	if *wipe {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			log.Fatal(err)
		}
		log.Printf("wiped storage: %s", dbPath)
		return
	}

	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
	}

	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := ensureSchema(db); err != nil {
		log.Fatal(err)
	}

	if *printMode {
		dumpSnapshots(db)
		return
	}

	if *analyzeMode {
		recs, err := analyze(db, time.Now())
		if err != nil {
			log.Printf("analysis: %v", err)
			return
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", "name", "current", "buy", "sell", "spread%")
		for _, r := range recs {
			spread := 0.0
			if r.BuyTarget > 0 {
				spread = (r.SellTarget - r.BuyTarget) / r.BuyTarget * 100
			}
			fmt.Printf("%s\t%.4f\t%.4f\t%.4f\t%.2f\n", r.Name, r.Current, r.BuyTarget, r.SellTarget, spread)
		}
		return
	}

	if *backtestMode {
		runBacktest(client, league)
		return
	}

	// Store hourly forever, analyze every 4h.
	webhookURLs := splitWebhookURLs(os.Getenv("DISCORD_WEBHOOK_URL"))

	if *sendMode {
		if len(webhookURLs) == 0 {
			log.Fatal("DISCORD_WEBHOOK_URL is required for -send")
		}
		analyzeAndSend(db, webhookURLs)
		return
	}

	if *testSendMode {
		if len(webhookURLs) == 0 {
			log.Fatal("DISCORD_WEBHOOK_URL is required for -send-test")
		}
		sample := []recommendation{
			{Name: "divine", Display: "Divine Orb", Current: 423.15, BuyTarget: 376.47, SellTarget: 422.94, BulkSell: 422.94},
			{Name: "exalted", Display: "Exalted Orb", Current: 142.80, BuyTarget: 119.21, SellTarget: 143.03, BulkSell: 143.03},
			{Name: "chaos", Display: "Chaos Orb", Current: 44.50, BuyTarget: 40.00, SellTarget: 48.00, BulkSell: 48.00},
			{Name: "mirror", Display: "Mirror of Kalandra", Current: 1767897.60, BuyTarget: 1600000.00, SellTarget: 1800000.00, BulkSell: 1800000.00},
		}
		if err := sendAnalysisDiscord(webhookURLs, sample, time.Now()); err != nil {
			log.Fatal(err)
		}
		log.Println("sent sample PNG to discord")
		return
	}

	go storeHourly(db, client, league)
	analyzeEvery(db, webhookURLs)
}

// splitWebhookURLs parses a comma-separated DISCORD_WEBHOOK_URL value into
// individual URLs, trimming whitespace and dropping empty entries.
func splitWebhookURLs(raw string) []string {
	parts := strings.Split(raw, ",")
	urls := make([]string, 0, len(parts))
	for _, p := range parts {
		if u := strings.TrimSpace(p); u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

// analyzeEvery runs the analysis and sends to Discord on a 4-hour cycle.
// If no webhook URL is set there is nothing to send, so it just blocks.
func analyzeEvery(db *sql.DB, webhookURLs []string) {
	if len(webhookURLs) == 0 {
		log.Println("no DISCORD_WEBHOOK_URL set; scheduled analysis disabled")
		select {}
	}
	ticker := time.NewTicker(analyzeInterval)
	defer ticker.Stop()
	for {
		analyzeAndSend(db, webhookURLs)
		<-ticker.C
	}
}

// dumpSnapshots prints every stored snapshot
func dumpSnapshots(db *sql.DB) {
	rows, err := db.Query(`SELECT name, price, quantity, ts FROM price_snapshots ORDER BY ts, name`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var (
		name     string
		price    float64
		quantity int64
		ts       time.Time
	)
	for rows.Next() {
		if err := rows.Scan(&name, &price, &quantity, &ts); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\t%f\t%d\t%s\n", name, price, quantity, ts.Format(time.RFC3339))
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
}

// storeHourly stores a snapshot immediately, then once every hour.
func storeHourly(db *sql.DB, client *http.Client, league string) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := storeSnapshot(db, client, league); err != nil {
			log.Printf("store snapshot: %v", err)
		}
		<-ticker.C
	}
}

// storeSnapshot fetches all currencies and inserts them as an hourly snapshot.
func storeSnapshot(db *sql.DB, client *http.Client, league string) error {
	data, err := apiCalls.CallApi(client, league, 1)
	if err != nil {
		return err
	}
	ts := time.Now().Truncate(time.Hour)
	for _, it := range data.Items {
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO price_snapshots (name, price, quantity, ts, text) VALUES (?, ?, ?, ?, ?)`,
			it.Name, it.CurrentPrice, it.CurrentQuantity, ts, it.Text,
		); err != nil {
			return err
		}
	}
	log.Printf("stored %d currency snapshots at %s", len(data.Items), ts.Format(time.RFC3339))
	return nil
}

// ensureSchema creates the price_snapshots table if it doesn't exist, then
// migrates older tables to have the display-name (text) column.
func ensureSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS price_snapshots (
			name     VARCHAR,
			price    DOUBLE,
			quantity BIGINT,
			ts       TIMESTAMP,
			text     VARCHAR,
			PRIMARY KEY (name, ts)
		)
	`); err != nil {
		return err
	}
	// Migration for DBs created before the text column existed.
	if _, err := db.Exec(`ALTER TABLE price_snapshots ADD COLUMN IF NOT EXISTS text VARCHAR`); err != nil {
		return err
	}
	return nil
}
