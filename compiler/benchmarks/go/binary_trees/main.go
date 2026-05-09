package main

import "fmt"

type TreeNode struct{ value int }

func pow2(exp int) int {
	out := 1
	for i := 0; i <= exp-1; i++ {
		out *= 2
	}
	return out
}

func buildTree(depth int) []TreeNode {
	nodes := []TreeNode{}
	total := pow2(depth+1) - 1
	for i := 0; i <= total; i++ {
		nodes = append(nodes, TreeNode{value: i + 1})
	}
	return nodes
}

func check(nodes []TreeNode, idx int) int {
	if idx >= len(nodes) {
		return 0
	}
	return nodes[idx].value + check(nodes, idx*2+1) + check(nodes, idx*2+2)
}

func main() {
	checksum := 0
	for depth := 5; depth <= 13; depth++ {
		tree := buildTree(depth)
		checksum += check(tree, 0) * depth
	}
	fmt.Print(checksum)
}
