package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BudgetID is the ID of the YNAB budget to query - Schumann Family Finances
const BudgetID = "ca2dc90d-7591-4a7b-ae87-7e84943ddd6c"

const Chase3577AccountID = "0ee0af62-bccc-41b9-9509-39f06707ea7e"
const ChaseFreedomAccountID = "5ad18e13-61a8-467b-92c8-ec86f7d6efdd"
const WamuAccountID = "9d0bd1ab-935c-4568-a4d6-8316692d8876"
const Chase2AccountID = "9b62d4c8-37ee-440a-b510-b32619e33da8"

type TimeSeries struct {
	Target     string      `json:"target"`
	DataPoints [][]float64 `json:"datapoints"`
}

type QueryRequest struct {
	Targets []struct {
		Target string `json:"target"`
	} `json:"targets"`
	Range struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	} `json:"range"`
}

type QueryResponse []TimeSeries

func queryHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	fmt.Printf("*************\nReq: %s\nHeaders: %+v\nBody: %s\n", r.RequestURI, r.Header, string(body))

	var req QueryRequest
	json.NewDecoder(bytes.NewBuffer(body)).Decode(&req)

	var resp QueryResponse
	for _, target := range req.Targets {
		resp = append(resp, TimeSeries{
			Target: target.Target,
			DataPoints: [][]float64{
				{100.0, float64(time.Now().Add(-time.Hour).Unix() * 1000)},
				{200.0, float64(time.Now().Unix() * 1000)},
			},
		})
	}

	json.NewEncoder(w).Encode(resp)
}

func balancesHandler(w http.ResponseWriter, r *http.Request) {
	// create a request to the ynab API
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.ynab.com/v1/budgets/%s", BudgetID), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// extract the bearer token from the request header
	token := r.Header.Get("Authorization")
	req.Header.Set("Authorization", token)

	// make the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	type YNABBudgetResp struct {
		Data struct {
			Budget struct {
				Accounts []struct {
					Name    string  `json:"name"`
					Balance float64 `json:"balance"`
				} `json:"accounts"`
			} `json:"budget"`
		} `json:"data"`
	}

	ynabBudgetResp := YNABBudgetResp{}
	err = json.NewDecoder(resp.Body).Decode(&ynabBudgetResp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type Balance struct {
		Name   string `json:"name"`
		Amount string `json:"amount"`
	}

	var balances []Balance
	for _, account := range ynabBudgetResp.Data.Budget.Accounts {
		balances = append(balances, Balance{
			Name:   account.Name,
			Amount: fmt.Sprintf("$%.2f", account.Balance/1000.0), // convert to dollars
		})
	}

	// Add in the x-rate-limit header as an additional "account"
	balances = append(balances, Balance{
		Name:   "YNAB API Rate Limit",
		Amount: resp.Header.Get("X-Rate-Limit"),
	})

	err = json.NewEncoder(w).Encode(balances)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func main() {
	http.HandleFunc("/balances", balancesHandler)
	http.HandleFunc("/query", queryHandler)
	fmt.Println("Listening on :8080")
	http.ListenAndServe(":8080", nil)
}
