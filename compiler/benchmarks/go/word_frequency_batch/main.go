package main

import (
	"fmt"
	"sort"
)

type WordCount struct {
	word  string
	count int
}

func expandWords(rounds int) []string {
	base := []string{"ard", "go", "vm", "parser", "checker", "runtime", "async", "trait", "result", "maybe"}

	words := make([]string, 0, (rounds+1)*3)
	for i := 0; i <= rounds; i++ {
		words = append(words, base[i%len(base)])
		words = append(words, base[(i+3)%len(base)])
		words = append(words, base[(i+7)%len(base)])
	}

	return words
}

func countWords(words []string) []WordCount {
	counts := make(map[string]int)

	for _, word := range words {
		counts[word]++
	}

	res := make([]WordCount, 0, len(counts))
	for word, count := range counts {
		res = append(res, WordCount{word: word, count: count})
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].count > res[j].count
	})

	return res
}

func main() {
	words := expandWords(40000)
	counts := countWords(words)

	checksum := 0
	for idx, entry := range counts {
		checksum += (idx + 1) * entry.count
		checksum += len(entry.word)
	}

	fmt.Print(checksum)
}
