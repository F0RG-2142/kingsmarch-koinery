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
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

func main() {
	printMode := flag.Bool("print", false, "print all stored snapshots and exit")
	wipe := flag.Bool("wipe", false, "delete the storage file and exit")
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

	// Store hourly forever.
	storeHourly(db, client, league)
}

// dumpSnapshots prints every stored snapshot (name, price, quantity, ts).
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
	items, err := apiCalls.FetchAll(client, league)
	if err != nil {
		return err
	}
	ts := time.Now().Truncate(time.Hour)
	for _, it := range items {
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO price_snapshots (name, price, quantity, ts) VALUES (?, ?, ?, ?)`,
			it.Name, it.CurrentPrice, it.CurrentQuantity, ts,
		); err != nil {
			return err
		}
	}
	log.Printf("stored %d currency snapshots at %s", len(items), ts.Format(time.RFC3339))
	return nil
}

// ensureSchema creates the price_snapshots table if it doesn't exist.
func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS price_snapshots (
			name     VARCHAR,
			price    DOUBLE,
			quantity BIGINT,
			ts       TIMESTAMP,
			PRIMARY KEY (name, ts)
		)
	`)
	return err
}
