package main

import (
	"fmt"
	"sync"
)

func heavySum(start, stop int) int {
	total := 0
	for i := start; i <= stop; i++ {
		total += i * i % 97
		total += i * 3 % 17
	}
	return total
}

func main() {
	results := make([]int, 25)
	var wg sync.WaitGroup

	for batch := 0; batch <= 24; batch++ {
		start := batch * 3000
		stop := start + 3000
		wg.Add(1)
		go func(batch, start, stop int) {
			defer wg.Done()
			results[batch] = heavySum(start, stop)
		}(batch, start, stop)
	}

	wg.Wait()

	checksum := 0
	for idx, result := range results {
		checksum += (idx + 1) * result
	}

	fmt.Print(checksum)
}
