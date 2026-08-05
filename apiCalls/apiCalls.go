package apiCalls

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// BaseURL is the poe2scout API root. Exported so tests can point it at a mock server.
var BaseURL = "https://api.poe2scout.com/poe2/Leagues"

// priceLog is one entry in a currency's daily price history (PriceLogs).
type priceLog struct {
	Price    float64 `json:"Price"`
	Time     string  `json:"Time"`
	Quantity int64   `json:"Quantity"`
}

type currencyItems struct {
	UniqueItemId    int        `json:"UniqueItemId"`
	ItemId          int        `json:"ItemId"`
	Name            string     `json:"ApiId"`
	Text            string     `json:"Text"`
	CurrentPrice    float64    `json:"CurrentPrice"`
	CurrentQuantity int32      `json:"CurrentQuantity"`
	PriceLogs       []priceLog `json:"PriceLogs"`
}

type currencyData struct {
	Total int             `json:"Total"`
	Pages float32         `json:"Pages"`
	Items []currencyItems `json:"Items"`
}

// CallApi fetches a single page of results with custom headers (e.g. User-Agent)
func CallApi(client *http.Client, leagueName string, pageNumber int) (currencyData, error) {
	reqUrl := fmt.Sprintf("%s/%s/Currencies/ByCategory?category=currency&pageNumber=%d&pageSize=100", BaseURL, leagueName, pageNumber)

	req, err := http.NewRequest(http.MethodGet, reqUrl, nil)
	if err != nil {
		return currencyData{}, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("User-Agent", "Poe2ScoutGoClient/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return currencyData{}, fmt.Errorf("error executing GET request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return currencyData{}, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return currencyData{}, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var respStruct currencyData
	if err := json.Unmarshal(bodyBytes, &respStruct); err != nil {
		return currencyData{}, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return respStruct, nil
}
