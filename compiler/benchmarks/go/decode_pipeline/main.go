package main

import (
	"encoding/json"
	"fmt"
)

type payloadData struct {
	Units   []int          `json:"units"`
	Weights []int          `json:"weights"`
	Counts  map[string]int `json:"counts"`
}

func decodeOnce(payload string) int {
	var raw payloadData
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		panic(err)
	}

	total := 0
	for idx, unit := range raw.Units {
		total += unit * raw.Weights[idx]
	}

	for _, count := range raw.Counts {
		total += count
	}

	return total
}

func main() {
	payload := `{"units":[1,3,5,7,9,11,13,15,17,19,21,23],"weights":[2,4,6,8,10,12,14,16,18,20,22,24],"counts":{"alpha":3,"beta":5,"gamma":8,"delta":13}}`
	checksum := 0

	for i := 0; i <= 12000; i++ {
		checksum += decodeOnce(payload)
	}

	fmt.Print(checksum)
}
