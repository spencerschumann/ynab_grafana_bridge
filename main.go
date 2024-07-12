package main

import (
	"encoding/json"
	"net/http"
	"time"
)

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
	var req QueryRequest
	json.NewDecoder(r.Body).Decode(&req)

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

func main() {
	http.HandleFunc("/query", queryHandler)
	http.ListenAndServe(":8080", nil)
}
