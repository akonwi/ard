package main

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func buildPayload(i int) string { return fmt.Sprintf("row:%d:%d:ard-benchmark", i, i*17%1009) }

func runBatch(count int) int {
	checksum := 0
	for i := 0; i <= count; i++ {
		payload := buildPayload(i)
		encoded := base64.StdEncoding.EncodeToString([]byte(payload))
		decodedBytes, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			panic(err)
		}
		decoded := string(decodedBytes)
		encodedURL := base64.RawURLEncoding.EncodeToString([]byte(decoded))
		decodedURLBytes, err := base64.RawURLEncoding.DecodeString(encodedURL)
		if err != nil {
			panic(err)
		}
		decodedURL := string(decodedURLBytes)
		checksum += len(encoded) + len(decodedURL)
		if strings.Contains(decodedURL, "ard") {
			checksum += i % 97
		}
	}
	return checksum
}

func main() { fmt.Print(runBatch(12000)) }
