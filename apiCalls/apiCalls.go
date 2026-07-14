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

type currencyPair struct {
	Data []currencyPairData `json:"Data"`
}

type currencyPairData struct {
	Epoch     int     `json:"Epoch"`
	MarketCap float32 `json:"MarketCap"`
	Volume    int     `json:"Volume"`
}

func CallApi(client *http.Client, leagueName, currencyName1, currencyName2 string, limit int32) ([]currencyPairData, error) {
	item1Id, err := findItemId(client, leagueName, currencyName1)
	if err != nil {
		fmt.Printf("Error finding item %s: %v\n", currencyName1, err)
		return []currencyPairData{}, nil
	}
	item2Id, err := findItemId(client, leagueName, currencyName2)
	if err != nil {
		fmt.Printf("Error finding item %s: %v\n", currencyName2, err)
		return []currencyPairData{}, nil
	}

	reqUrl := fmt.Sprintf("%s/%s/Currencies/Pairs/%d/%d/History", baseUrl, leagueName, item1Id, item2Id)
	fmt.Println(reqUrl)
	resp, err := client.Get(reqUrl)
	if err != nil {
		return []currencyPairData{}, fmt.Errorf("error executing GET request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []currencyPairData{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return []currencyPairData{}, fmt.Errorf("error reading the response body: %w", err)
	}

	var respStruct currencyPair

	if json.Unmarshal(bodyBytes, &respStruct) != nil {
		return []currencyPairData{}, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return respStruct.Data, nil
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
