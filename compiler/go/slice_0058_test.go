package gotarget

import "testing"

func TestGoTargetCheckedSlicing(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "list slices share visible elements and nest",
			input: `fn main() Bool {
  let values = mut [10, 20, 30, 40]
  let view = values.slice(start: 1, end: 4).expect("bounds")
  let nested = view.slice(start: 1).expect("bounds")
  let writable = mut nested
  let changed = writable.set(0, 99)
  let rejected = not writable.set(9, 77)
  writable.swap(0, 1)
  changed and rejected and values.at(2).or(0) == 40 and values.at(3).or(0) == 99 and view.size() == 3
}`,
			want: "true",
		},
		{
			name: "source growth detaches from capped view",
			input: `fn main() Bool {
  let values = mut [1, 2]
  let view = values.slice().expect("bounds")
  values.push(3)
  values.set(0, 9)
  values.size() == 3 and values.at(0).or(0) == 9 and view.size() == 2 and view.at(0).or(0) == 1
}`,
			want: "true",
		},
		{
			name: "to_list copies visible storage",
			input: `fn main() Bool {
  let values = mut [10, 20, 30]
  let view = values.slice(start: 1).expect("bounds")
  let copy = mut view.to_list()
  copy.set(0, 99)
  values.at(1).or(0) == 20 and view.at(0).or(0) == 20 and copy.at(0).or(0) == 99
}`,
			want: "true",
		},
		{
			name: "at returns none when no element exists at index",
			input: `fn main() Bool {
  let values: [Int] = []
  let view = values.slice().expect("bounds")
  values.at(0).is_none() and view.at(0).is_none()
}`,
			want: "true",
		},
		{
			name: "nullable defaults empty ranges and invalid bounds",
			input: `fn main() Bool {
  let values = [10, 20, 30]
  let missing: Int? = Maybe::new()
  let full = values.slice(start: missing, end: missing).expect("full")
  let empty = values.slice(start: 3).expect("empty")
  full.size() == 3 and empty.is_empty() and values.slice(start: -1).is_none() and values.slice(start: 2, end: 1).is_none() and values.slice(end: 4).is_none()
}`,
			want: "true",
		},
		{
			name: "named bounds evaluate in source order",
			input: `fn mark(log: mut [Int], value: Int) Int {
  log.push(value)
  value
}

fn main() Bool {
  let values = [10, 20, 30]
  let list_log: mut [Int] = mut []
  let view = values.slice(end: mark(list_log, 3), start: mark(list_log, 1)).expect("bounds")
  let string_log: mut [Int] = mut []
  let text = "abc".slice(end: mark(string_log, 3), start: mark(string_log, 1)).expect("bounds")
  view.size() == 2 and text == "bc" and list_log.at(0).or(0) == 3 and list_log.at(1).or(0) == 1 and string_log.at(0).or(0) == 3 and string_log.at(1).or(0) == 1
}`,
			want: "true",
		},
		{
			name: "slice iteration",
			input: `fn main() Int {
  let values = [10, 20, 30, 40]
  let view = values.slice(start: 1, end: 3).expect("bounds")
  mut total = 0
  for value in view {
    total = total + value
  }
  total
}`,
			want: "50",
		},
		{
			name: "generic functions preserve Slice identity",
			input: `fn tail(view: Slice<$T>) Slice<$T>? {
  view.slice(start: 1)
}

fn main() Int {
  let view = [10, 20, 30].slice().expect("bounds")
  tail(view).expect("tail").at(0).or(0)
}`,
			want: "20",
		},
		{
			name: "string slices use byte offsets",
			input: `fn main() Bool {
  let text = "héllo"
  let whole = text.slice().expect("whole")
  let accent = text.slice(start: 1, end: 3).expect("accent")
  let split = text.slice(start: 1, end: 2).expect("split")
  whole == text and accent == "é" and split.size() == 1 and text.slice(start: -1).is_none() and text.slice(end: 7).is_none()
}`,
			want: "true",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := runGoTargetParityJSON(t, program); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
