package main

import (
	"fmt"
	"sort"
)

type Square struct {
	size int
}

type Circle struct {
	radius int
}

type Triangle struct {
	base   int
	height int
}

type Hexagon struct {
	side int
}

type Shape interface {
	shape()
}

func (Square) shape()   {}
func (Circle) shape()   {}
func (Triangle) shape() {}
func (Hexagon) shape()  {}

type ShapeStat struct {
	kind  string
	total int
	seen  int
}

func buildShapes(count int) []Shape {
	shapes := make([]Shape, 0, count+1)

	for i := 0; i <= count; i++ {
		switch i % 4 {
		case 0:
			shapes = append(shapes, Square{size: i % (23 + 2)})
		case 1:
			shapes = append(shapes, Circle{radius: i % (19 + 3)})
		case 2:
			shapes = append(shapes, Triangle{base: i % (17 + 4), height: i % (13 + 5)})
		default:
			shapes = append(shapes, Hexagon{side: i % (11 + 6)})
		}
	}

	return shapes
}

func label(shape Shape) string {
	switch shape.(type) {
	case Square:
		return "square"
	case Circle:
		return "circle"
	case Triangle:
		return "triangle"
	case Hexagon:
		return "hexagon"
	default:
		return ""
	}
}

func score(shape Shape) int {
	switch s := shape.(type) {
	case Square:
		return s.size * s.size
	case Circle:
		return s.radius * s.radius * 3
	case Triangle:
		return s.base * s.height / 2
	case Hexagon:
		return s.side * s.side * 2
	default:
		return 0
	}
}

func summarize(shapes []Shape) []ShapeStat {
	totals := make(map[string]int)
	counts := make(map[string]int)

	for _, shape := range shapes {
		kind := label(shape)
		totals[kind] += score(shape)
		counts[kind]++
	}

	stats := make([]ShapeStat, 0, len(totals))
	for kind, total := range totals {
		stats = append(stats, ShapeStat{
			kind:  kind,
			total: total,
			seen:  counts[kind],
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].total > stats[j].total
	})

	return stats
}

func main() {
	shapes := buildShapes(50000)
	stats := summarize(shapes)

	checksum := 0
	for idx, stat := range stats {
		checksum += (idx + 1) * stat.total
		checksum += stat.seen
	}

	fmt.Print(checksum)
}
