package gotarget

import "testing"

func TestGoTargetFixedArrayLiteral(t *testing.T) {
	program := lowerParitySource(t, `fn main() Bool {
  let rgb: [Byte; 3] = [255, 0, 1]
  rgb.size() == 3
}`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}

func TestGoTargetFixedArrayAt(t *testing.T) {
	program := lowerParitySource(t, `fn main() Bool {
  let xs: [Int; 3] = [10, 20, 30]
  xs.at(1).or(0) == 20 and xs.at(3).is_none()
}`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}

func TestGoTargetFixedArrayIteration(t *testing.T) {
	program := lowerParitySource(t, `fn main() Int {
  let xs: [Int; 3] = [1, 2, 3]
  mut sum = 0
  for x in xs {
    sum = sum + x
  }
  sum
}`)
	if got := runGoTargetParityJSON(t, program); got != "6" {
		t.Fatalf("got %s, want 6", got)
	}
}
