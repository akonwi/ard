package main

import (
	"fmt"
	"strings"
)

func buildDNA(size int) []string {
	bases := []string{"A", "C", "G", "T", "G", "A", "T", "C", "A", "G", "G", "T", "C"}
	dna := make([]string, 0, size+1)
	for i := 0; i <= size; i++ {
		dna = append(dna, bases[(i*7+i/3)%len(bases)])
	}
	return dna
}

func countKmers(dna []string) map[string]int {
	counts := map[string]int{}
	for i := 0; i <= len(dna)-3; i++ {
		kmer := dna[i] + dna[i+1] + dna[i+2]
		counts[kmer]++
	}
	return counts
}

func checksum(counts map[string]int) int {
	total := 0
	for kmer, count := range counts {
		total += len(kmer) * count
		if strings.Contains(kmer, "A") {
			total += count * 7
		}
		if strings.HasPrefix(kmer, "G") {
			total += count * 11
		}
	}
	return total
}

func main() { fmt.Print(checksum(countKmers(buildDNA(80000)))) }
