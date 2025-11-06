package checker

import "testing"

func TestTypeEquality(t *testing.T) {
	var tests = []struct {
		left   Type
		right  Type
		expect bool
	}{
		{
			left:   &Any{name: "T"},
			right:  Str,
			expect: true,
		},
		{
			left:   Str,
			right:  &Any{name: "T"},
			expect: true,
		},
		{
			left:   MakeResult(&Any{name: "T"}, Void),
			right:  MakeResult(Str, Void),
			expect: true,
		},
		{
			left:   MakeResult(Str, Void),
			right:  MakeResult(&Any{name: "T"}, Void),
			expect: true,
		},
		{
			left: &FunctionDef{
				Parameters: []Parameter{},
				ReturnType: MakeResult(Str, Void),
			},
			right: &FunctionDef{
				Parameters: []Parameter{},
				ReturnType: MakeResult(&Any{name: "T"}, Void),
			},
			expect: true,
		},
		{
			left: &FunctionDef{
				Parameters: []Parameter{},
				ReturnType: MakeResult(Str, Void),
			},
			right: &FunctionDef{
				Parameters: []Parameter{},
				ReturnType: MakeResult(&Any{name: "T"}, Void),
			},
			expect: true,
		},
		{
			left: &ExternalFunctionDef{
				Parameters: []Parameter{
					{
						Name: "foo",
						Type: &Any{name: "T"},
					},
				},
				ReturnType: MakeResult(Str, Void),
			},
			right: &FunctionDef{
				Parameters: []Parameter{
					{
						Name: "foo",
						Type: Int,
					},
				},
				ReturnType: MakeResult(&Any{name: "T"}, Void),
			},
			expect: true,
		},
		{
			left: &FunctionDef{
				Parameters: []Parameter{
					{
						Name: "foo",
						Type: Int,
					},
				},
				ReturnType: Void,
			},
			right: &ExternalFunctionDef{
				Parameters: []Parameter{
					{
						Name: "foo",
						Type: &Any{name: "T"},
					},
				},
				ReturnType: Void,
			},
			expect: true,
		},
	}

	for _, test := range tests {
		got := test.left.equal(test.right)
		if got != test.expect {
			t.Errorf("%s == %s: got %v, want %v", test.left, test.right, got, test.expect)
		}
	}
}
