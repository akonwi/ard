package main

import "fmt"

func kernel(size int) int {
	checksum := 0
	energy := 0.5
	for i := 1; i <= size; i++ {
		row := 0
		for j := 1; j <= 64; j++ {
			value := (i*j + (i%17)*(j%13)) % 1009
			row += value * (j%7 + 1)
			energy += 1.25 / 2.5
		}
		checksum += row % 1000003
	}
	if energy > 0 {
		checksum++
	}
	return checksum
}

func main() { fmt.Print(kernel(9000)) }
