package apiCalls

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var baseUrl = "https://api.poe2scout.com/poe2/Leagues"

type currencyID struct {
	CurrencyItemId int `json:"CurrencyItemID"`
	ItemId         int `json:"ItemID"`
}

type PriceLogs struct {
	Price    float32 `json:"Price"`
	Time     string  `json:"Time"`
	Quantity int     `json:"Quantity"`
}

type currencyItems struct {
	UniqueItemId    int         `json:"UniqueItemId"`
	ItemId          int         `json:"ItemId"`
	Name            string      `json:"ApiId"`
	PriceLogs       []PriceLogs `json:"PriceLogs"`
	CurrentPrice    float32     `json:"CurrentPrice"`
	CurrentQuantity int32       `json:"CurrentQuantity"`
}

type currencyData struct {
	Total int             `json:"Total"`
	Pages float32         `json:"Pages"`
	Items []currencyItems `json:"Items"`
}

// CallApi fetches a single page of results with custom headers (e.g. User-Agent)
func CallApi(client *http.Client, leagueName string, pageNumber int) (currencyData, error) {
	reqUrl := fmt.Sprintf("%s/%s/Currencies/ByCategory?category=currency&pageNumber=%d&pageSize=25", baseUrl, leagueName, pageNumber)

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

// FetchAllMonthlyData loops over all pages and filters PriceLogs to the last 30 days.
func FetchAllMonthlyData(client *http.Client, leagueName string) ([]currencyItems, error) {
	var allItems []currencyItems
	currentPage := 1
	totalPages := 1

	cutoffDate := time.Now().AddDate(0, 0, -30)

	for currentPage <= totalPages {
		data, err := CallApi(client, leagueName, currentPage)
		if err != nil {
			return nil, fmt.Errorf("failed fetching page %d: %w", currentPage, err)
		}
		if data.Pages > 0 {
			totalPages = int(data.Pages)
		}

		for _, item := range data.Items {
			var monthlyLogs []PriceLogs
			for _, log := range item.PriceLogs {
				logTime, err := time.Parse(time.RFC3339, log.Time)
				if err != nil {
					logTime, _ = time.Parse("2006-01-02T15:04:05", log.Time)
				}

				if logTime.After(cutoffDate) {
					monthlyLogs = append(monthlyLogs, log)
				}
			}

			item.PriceLogs = monthlyLogs
			allItems = append(allItems, item)
		}

		currentPage++
	}

	return allItems, nil
}
