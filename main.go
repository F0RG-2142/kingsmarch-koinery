package main

import (
	"database/sql"
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
	client := http.DefaultClient

	// Persistent DuckDB file so hourly snapshots survive restarts.
	dbPath := os.Getenv("DUCKDB_PATH")
	if dbPath == "" {
		dbPath = "./data/prices.duckdb"
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

	data, err := apiCalls.CallApi(client, "runes", 1)
	if err != nil {
		fmt.Printf("error calling data: %s", err)
		return
	}
	for _, v := range data.Items {
		fmt.Printf("%d %s/s for %f Exalted Orbs each at %v\n", v.CurrentQuantity, v.Name, v.CurrentPrice, time.Now())
	}
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
