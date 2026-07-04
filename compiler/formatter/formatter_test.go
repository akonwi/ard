package formatter

import "testing"

func TestFormatIsIdempotent(t *testing.T) {
	inputs := []struct {
		name  string
		input string
	}{
		{
			name:  "try catch block",
			input: "fn example() {\n  let raw = try self.raw -> _ {\n    \"\"\n  }\n  next(raw)\n}\n",
		},
		{
			name:  "test function",
			input: "test fn my_test() Void!Str {\n  try testing::assert(true, \"ok\")\n  Result::ok(())\n}\n",
		},
		{
			name:  "test and regular functions together",
			input: "fn helper() Int {\n  1\n}\ntest fn test_helper() Void!Str {\n  Result::ok(())\n}\n",
		},
		{
			name:  "anonymous function with inferred parameter type",
			input: "fn main() {\n  let adults = List::keep(\n    users,\n    fn(u) {\n      u.age >= 30\n    },\n  )\n}\n",
		},
		{
			name:  "go import",
			input: "use go:fmt\n\nfn main() {\n  fmt::Println(\"hello\")\n}\n",
		},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			first, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("first format failed: %v", err)
			}

			second, err := Format(first, "test.ard")
			if err != nil {
				t.Fatalf("second format failed: %v", err)
			}

			if string(first) != string(second) {
				t.Fatalf("expected idempotent formatting, first=%q second=%q", string(first), string(second))
			}
		})
	}
}

func TestFormatGenericStructLiterals(t *testing.T) {
	inputs := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "one-line generic struct literal keeps type args",
			input: "fn main() {\n  let r = Radio<Str>{value: \"compact\"}\n}\n",
			want:  "fn main() {\n  let r = Radio<Str>{value: \"compact\"}\n}\n",
		},
		{
			name:  "empty generic struct literal keeps type args",
			input: "fn main() {\n  let b = Box<Int>{}\n}\n",
			want:  "fn main() {\n  let b = Box<Int>{}\n}\n",
		},
		{
			name:  "multi-line generic struct literal keeps type args",
			input: "fn main() {\n  let r = Radio<Str>{value: \"compact\", group: mode, label: \"Compact\", disabled: false}\n}\n",
			want:  "fn main() {\n  let r = Radio<Str>{\n    value: \"compact\",\n    group: mode,\n    label: \"Compact\",\n    disabled: false,\n  }\n}\n",
		},
		{
			name:  "static generic struct literal keeps type args",
			input: "fn main() {\n  let p = ui::Provider<ui::Theme>{value: active}\n}\n",
			want:  "fn main() {\n  let p = ui::Provider<ui::Theme>{value: active}\n}\n",
		},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("format failed: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", got, tt.want)
			}

			again, err := Format(got, "test.ard")
			if err != nil {
				t.Fatalf("second format failed: %v", err)
			}
			if string(again) != string(got) {
				t.Fatalf("format not idempotent:\nfirst:\n%s\nsecond:\n%s", got, again)
			}
		})
	}
}
