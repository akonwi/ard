package main

import "fmt"

type Cache struct {
	capacity int
	values   map[int]int
	order    []int
}

func removeKey(order []int, key int) []int {
	out := make([]int, 0, len(order))
	for _, item := range order {
		if item != key {
			out = append(out, item)
		}
	}
	return out
}

func cachePut(cache Cache, key, value int) Cache {
	values := cache.values
	order := removeKey(cache.order, key)
	if _, ok := values[key]; !ok && len(order) >= cache.capacity {
		evicted := order[0]
		order = append([]int{}, order[1:]...)
		delete(values, evicted)
	}
	values[key] = value
	order = append(order, key)
	return Cache{capacity: cache.capacity, values: values, order: order}
}

func runCache(rounds int) int {
	cache := Cache{capacity: 128, values: map[int]int{}, order: []int{}}
	checksum := 0
	for i := 0; i <= rounds; i++ {
		key := (i*37 + i/3) % 257
		if value, ok := cache.values[key]; ok {
			checksum += value
		} else {
			checksum += key
		}
		cache = cachePut(cache, key, i*11+key)
	}
	return checksum + len(cache.order) + len(cache.values)
}

func main() { fmt.Print(runCache(12000)) }
