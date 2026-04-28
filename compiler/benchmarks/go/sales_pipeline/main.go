package main

import (
	"fmt"
	"sort"
)

type Region int

const (
	North Region = iota
	South
	East
	West
)

func (r Region) weight() int {
	switch r {
	case North:
		return 3
	case South:
		return 2
	case East:
		return 4
	case West:
		return 5
	default:
		return 0
	}
}

type Sale struct {
	rep    string
	region Region
	units  int
	price  int
}

func (s Sale) revenue() int {
	return s.units * s.price * s.region.weight()
}

type RepTotal struct {
	rep     string
	revenue int
	volume  int
}

func buildSales(count int) []Sale {
	reps := []string{"Ava", "Ben", "Cy", "Di", "Eli", "Fae"}
	sales := make([]Sale, 0, count+1)

	for i := 0; i <= count; i++ {
		region := North
		switch i % 4 {
		case 1:
			region = South
		case 2:
			region = East
		case 3:
			region = West
		}

		sales = append(sales, Sale{
			rep:    reps[i%len(reps)],
			region: region,
			units:  i % (11 + 1),
			price:  i * 7 % (13 + 5),
		})
	}

	return sales
}

func summarize(sales []Sale) []RepTotal {
	revenueByRep := make(map[string]int)
	volumeByRep := make(map[string]int)

	for _, sale := range sales {
		revenueByRep[sale.rep] += sale.revenue()
		volumeByRep[sale.rep] += sale.units
	}

	totals := make([]RepTotal, 0, len(revenueByRep))
	for rep, revenue := range revenueByRep {
		totals = append(totals, RepTotal{
			rep:     rep,
			revenue: revenue,
			volume:  volumeByRep[rep],
		})
	}

	sort.Slice(totals, func(i, j int) bool {
		a := totals[i]
		b := totals[j]
		return a.revenue*1000+a.volume > b.revenue*1000+b.volume
	})

	return totals
}

func main() {
	sales := buildSales(40000)
	totals := summarize(sales)

	checksum := 0
	for idx, total := range totals {
		checksum += (idx + 1) * total.revenue
		checksum += total.volume
	}

	fmt.Print(checksum)
}
