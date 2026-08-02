package main

import (
	"database/sql"
	"fmt"
	"kingsmarch-koinery/apiCalls"
	"log"
	"net/http"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

type CurrencyData struct {
}

// TokenResponse matches the JSON structure returned by the PoE OAuth server
func main() {
	//Connect everything
	client := http.DefaultClient

	db, err := sql.Open("duckdb", "")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	data, err := apiCalls.CallApi(client, "runes", 1)
	if err != nil {
		fmt.Printf("error calling data: %s", err)
		return
	}
	for _, v := range data.Items {
		fmt.Printf("%d %s/s for %f Exalted Orbs each at %v\n", v.CurrentQuantity, v.Name, v.CurrentPrice, time.Now())
	}
}
