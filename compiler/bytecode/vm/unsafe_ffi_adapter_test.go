package vm

import "testing"

func TestValueNativeVMToLegacyFFIRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "scalar extern result feeds native arithmetic",
			input: `
				let value = Float::from_int(3)
				value + 2.5
			`,
			want: 5.5,
		},
		{
			name: "maybe extern result feeds native maybe ops and arithmetic",
			input: `
				let parsed = Int::from_str("40")
				parsed.or(0) + 2
			`,
			want: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runBytecode(t, tt.input)
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
