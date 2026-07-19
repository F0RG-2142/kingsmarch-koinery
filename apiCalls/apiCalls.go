package apiCalls

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Name            string      `json:"Name"`
	Type            string      `json:"Type"`
	PriceLogs       []PriceLogs `json:"PriceLogs"`
	CurrentPrice    float32     `json:"CurrentPrice"`
	CurrentQuantity int32       `json:"CurrentQuantity"`
}

type currencyData struct {
	Total int             `json:"Total"`
	Pages float32         `json:"Pages"`
	Items []currencyItems `json:"Items"`
}

func CallApi(client *http.Client, leagueName string, pageNumber int32) ([]currencyItems, error) {

	reqUrl := fmt.Sprintf("%s/%s/Currencies/ByCategory?category=currency&pageNumber=%d&pageSize=25", baseUrl, leagueName, pageNumber)
	fmt.Println(reqUrl)
	resp, err := client.Get(reqUrl)
	if err != nil {
		return []currencyItems{}, fmt.Errorf("error executing GET request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return []currencyItems{}, fmt.Errorf("error reading the response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return []currencyItems{}, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var respStruct currencyData

	if json.Unmarshal(bodyBytes, &respStruct) != nil {
		return []currencyItems{}, fmt.Errorf("error unmarshaling response: %w", err)
	}

	if len(respStruct.Items) > 0 {
		fmt.Println(respStruct.Items[0])
	} else {
		fmt.Println("The request succeeded, but the items array is empty.")
	}

	fmt.Println(respStruct.Items[0])

	return respStruct.Items, nil
}

func findItemId(client *http.Client, leagueName, currencyName string) (int, error) {
	reqString := fmt.Sprintf("%s/%s/Currencies/%s", baseUrl, leagueName, currencyName)

	fmt.Println(reqString)

	resp, err := client.Get(reqString)
	if err != nil {
		return 0, fmt.Errorf("error executing GET request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("error reading response body: %w", err)
	}

	var respStruct currencyID

	err = json.Unmarshal(bodyBytes, &respStruct)
	if err != nil {
		return 0, fmt.Errorf("error unmarshalling JSON: %w", err)
	}

	return respStruct.ItemId, nil
}
