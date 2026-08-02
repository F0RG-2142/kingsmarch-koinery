package apiCalls

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var baseUrl = "https://api.poe2scout.com/poe2/Leagues"

type currencyItems struct {
	UniqueItemId    int     `json:"UniqueItemId"`
	ItemId          int     `json:"ItemId"`
	Name            string  `json:"ApiId"`
	CurrentPrice    float32 `json:"CurrentPrice"`
	CurrentQuantity int32   `json:"CurrentQuantity"`
}

type currencyData struct {
	Total int             `json:"Total"`
	Pages float32         `json:"Pages"`
	Items []currencyItems `json:"Items"`
}

// CallApi fetches a single page of results with custom headers (e.g. User-Agent)
func CallApi(client *http.Client, leagueName string, pageNumber int) (currencyData, error) {
	reqUrl := fmt.Sprintf("%s/%s/Currencies/ByCategory?category=currency&pageNumber=%d&pageSize=100", baseUrl, leagueName, pageNumber)

	req, err := http.NewRequest(http.MethodGet, reqUrl, nil)
	if err != nil {
		return currencyData{}, fmt.Errorf("error creating request: %w", err)
	}

	// Good practice for poe2scout API: include a User-Agent identifying your app
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

// FetchAll returns the full set of a league's currency items.
// Note: the poe2scout endpoint ignores pageNumber/pageSize and always returns
// the same single page of items, so there is nothing to paginate over.
func FetchAll(client *http.Client, leagueName string) ([]currencyItems, error) {
	first, err := CallApi(client, leagueName, 1)
	if err != nil {
		return nil, fmt.Errorf("error fetching currency data: %w", err)
	}
	return first.Items, nil
}
