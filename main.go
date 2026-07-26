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
	data, err := apiCalls.CallApi(client, "runes", 2)
	if err != nil {
		fmt.Printf("error calling data: %s", err)
		return
	}
	for _, v := range data {
		for _, p := range v.PriceLogs {
			fmt.Printf("%s: %f Exalted Orbs\n", v.Name, p.Price)
		}
	}
}
