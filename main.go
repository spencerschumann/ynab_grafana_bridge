package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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

func authorization(r *http.Request) string {
	return r.Header.Get("Authorization")
}

func ynabRequest(resp any, auth string, path string, args ...any) (*http.Response, error) {
	// create a request to the ynab API
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.ynab.com/v1/"+path, args...), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", auth)

	// make the request
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	err = json.NewDecoder(r.Body).Decode(resp)
	return r, err
}

// reportError reports an errer, if non-nil, to the client
// err is a pointer to the error to enable direct use with defer.
func reportError(w http.ResponseWriter, err *error) {
	if *err != nil {
		http.Error(w, (*err).Error(), http.StatusInternalServerError)
	}
}

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

type YNABAccountResp struct {
	Data struct {
		Account struct {
			Name             string  `json:"name"`
			Balance          float64 `json:"balance"`
			ClearedBalance   float64 `json:"cleared_balance"`
			UnclearedBalance float64 `json:"uncleared_balance"`
		} `json:"account"`
	} `json:"data"`
}

type YNABTransactionsResp struct {
	Data struct {
		Transactions []struct {
			Date     string  `json:"date"`
			Account  string  `json:"account_name"`
			Amount   float64 `json:"amount"`
			Payee    string  `json:"payee_name"`
			Category string  `json:"category_name"`
			// TODO: there are fields for transfers that may be relevant
		} `json:"transactions"`
	} `json:"data"`
}

type YNABScheduledTransactionsResp struct {
	Data struct {
		ScheduledTransactions []struct {
			Date string `json:"date_first"`

			// We're not currently using scheduled transctions, but if we ever do, these fields will be needed.
			DateNext  string `json:"date_next"`
			Frequency string `json:"frequency"`

			AccountID string  `json:"account_id"`
			Amount    float64 `json:"amount"`
		} `json:"scheduled_transactions"`
	} `json:"data"`
}

func balancesHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	defer reportError(w, &err)

	ynabBudgetResp := YNABBudgetResp{}
	resp, err := ynabRequest(&ynabBudgetResp, authorization(r), "budgets/%s", BudgetID)
	if err != nil {
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
}

func transactionsHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	defer reportError(w, &err)

	// could get this via a cached all accounts call, but this is simpler for now
	account := YNABAccountResp{}
	_, err = ynabRequest(&account, authorization(r),
		"budgets/%s/accounts/%s", BudgetID, Chase3577AccountID)
	if err != nil {
		return
	}

	ynabTransactions := YNABTransactionsResp{}
	// Just fetch Chase 3577 transactions for now
	// TODO: may want to cache these, and fetch transactions for all accounts
	// to reduce the number of API calls
	_, err = ynabRequest(&ynabTransactions, authorization(r),
		"budgets/%s/accounts/%s/transactions", BudgetID, Chase3577AccountID)
	if err != nil {
		return
	}
	// strip off the outer data objects
	transactions := ynabTransactions.Data.Transactions

	scheduledTransactions := YNABScheduledTransactionsResp{}
	_, err = ynabRequest(&scheduledTransactions, authorization(r),
		"budgets/%s/scheduled_transactions", BudgetID)
	if err != nil {
		return
	}

	type Transaction struct {
		Date string `json:"date"`
		//Amount  float64 `json:"amount"`
		Balance float64 `json:"balance"`
	}

	var resp []Transaction

	// Sort transactions new to old
	sort.SliceStable(transactions, func(i, j int) bool {
		return transactions[i].Date > transactions[j].Date
	})

	fmt.Printf("Balance: %f, transactions: %d, scheduled transactions: %d\n", account.Data.Account.Balance,
		len(transactions), len(scheduledTransactions.Data.ScheduledTransactions))

	// Calculate the running balance
	balance := account.Data.Account.Balance
	date := ""
	dailySum := 0.0
	for _, transaction := range transactions {
		// Sum all the transactions per date, then add them to the balance
		if date != transaction.Date {
			// Traveling backwards, so subtract
			balance -= dailySum
			dailySum = 0.0
			date = transaction.Date
			if date != "" {
				resp = append(resp, Transaction{
					Date:    date + "T00:00:00Z",
					Balance: balance / 1000,
				})
			}
		}
		dailySum += transaction.Amount
	}
	if date != "" {
		resp = append(resp, Transaction{
			Date:    date + "T00:00:00Z",
			Balance: balance / 1000,
		})
	}

	err = json.NewEncoder(w).Encode(resp)
}

func transactionsByPayeeHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	defer reportError(w, &err)

	ynabTransactions := YNABTransactionsResp{}
	// TODO: may want to cache these, and fetch transactions for all accounts
	// to reduce the number of API calls
	_, err = ynabRequest(&ynabTransactions, authorization(r),
		"budgets/%s/accounts/%s/transactions", BudgetID, Chase3577AccountID)
	if err != nil {
		return
	}
	// strip off the outer data objects
	transactions := ynabTransactions.Data.Transactions

	type Transaction struct {
		Date   string  `json:"date"`
		Amount float64 `json:"amount"`
		Payee  string  `json:"payee"`
	}

	var resp []Transaction

	// Summarize per month
	toMonth := func(date string) string {
		return date[:7] + "-01"
	}

	payeeCounts := map[string]int{}
	for _, transaction := range transactions {
		payeeCounts[transaction.Payee]++
	}

	grouped := map[string]map[string]float64{}
	for _, transaction := range transactions {
		month := toMonth(transaction.Date)
		if _, ok := grouped[month]; !ok {
			grouped[month] = map[string]float64{}
		}
		payee := transaction.Payee
		if payeeCounts[payee] < 2 {
			payee = "Misc"
		}
		grouped[month][payee] += transaction.Amount
	}

	for month, payees := range grouped {
		for payee, amount := range payees {
			resp = append(resp, Transaction{
				Date:   month + "T00:00:00Z",
				Amount: amount / 1000,
				Payee:  payee,
			})
		}
	}

	// Sort by date and payee
	sort.SliceStable(resp, func(i, j int) bool {
		if resp[i].Date == resp[j].Date {
			return resp[i].Payee < resp[j].Payee
		}
		return resp[i].Date < resp[j].Date
	})

	err = json.NewEncoder(w).Encode(resp)
}

func main() {
	http.HandleFunc("/balances", balancesHandler)
	http.HandleFunc("/transactions", transactionsHandler)
	http.HandleFunc("/transactions-by-payee", transactionsByPayeeHandler)
	http.HandleFunc("/query", queryHandler)
	fmt.Println("Listening on :8080")
	http.ListenAndServe(":8080", nil)
}
