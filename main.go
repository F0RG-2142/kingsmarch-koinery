package main

import (
	"fmt"
	"kingsmarch-koinery/apiCalls"
	"net/http"
)

// TokenResponse matches the JSON structure returned by the PoE OAuth server
func main() {
	client := http.DefaultClient
	data, err := apiCalls.CallApi(client, "runes", "chaos", "exalted", 100)
	if err != nil {
		fmt.Printf("error calling data: %s", err)
		return
	}
	for _, v := range data {
		fmt.Printf("%d", v.Epoch)
		fmt.Printf("%f", v.MarketCap)
		fmt.Printf("%d", v.Volume)
	}
}
