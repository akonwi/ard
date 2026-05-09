package main

import (
	"encoding/json"
	"fmt"
)

type Packet struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
	Values []int  `json:"values"`
}

func makePacket(i int) Packet {
	return Packet{ID: i, Name: fmt.Sprintf("packet-%d", i), Active: i%3 != 0, Values: []int{i, i * 2, i * 3, i % 17}}
}

func scorePacket(payload []byte) int {
	var p Packet
	if err := json.Unmarshal(payload, &p); err != nil {
		panic(err)
	}
	total := p.ID + len(p.Name)
	if p.Active {
		total += 19
	}
	for idx, value := range p.Values {
		total += value * (idx + 1)
	}
	return total
}

func main() {
	checksum := 0
	for i := 0; i <= 5000; i++ {
		encoded, err := json.Marshal(makePacket(i))
		if err != nil {
			panic(err)
		}
		checksum += scorePacket(encoded)
	}
	fmt.Print(checksum)
}
