package main

import (
	"fmt"
	"kingsmarch-koinery/apiCalls"
	"net/http"
)

type CurrencyData struct {
}

// TokenResponse matches the JSON structure returned by the PoE OAuth server
func main() {
	client := http.DefaultClient
	data, err := apiCalls.FetchAllMonthlyData(client, "runes")
	if err != nil {
		fmt.Printf("error calling data: %s", err)
		return
	}
	for _, v := range data {
		for _, p := range v.PriceLogs {
			fmt.Printf("%d %s/s for %f Exalted Orbs each, at %v\n", p.Quantity, v.Name, p.Price, p.Time)
		}
	}
}
