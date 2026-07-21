package air

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestLowerForeignResultShapeUsesExplicitAIRValues(t *testing.T) {
	tests := []struct {
		checker checker.ForeignResultShape
		want    ForeignResultShape
	}{
		{checker: checker.ForeignResultUnknown, want: ForeignResultUnknown},
		{checker: checker.ForeignResultDirect, want: ForeignResultDirect},
		{checker: checker.ForeignResultValueError, want: ForeignResultValueError},
		{checker: checker.ForeignResultErrorOnly, want: ForeignResultErrorOnly},
		{checker: checker.ForeignResultValueBool, want: ForeignResultValueBool},
	}

	for _, tt := range tests {
		if got := lowerForeignResultShape(tt.checker); got != tt.want {
			t.Errorf("lowerForeignResultShape(%v) = %v, want %v", tt.checker, got, tt.want)
		}
	}
}
