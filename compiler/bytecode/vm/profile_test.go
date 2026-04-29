package vm

import (
	"strings"
	"testing"
	"time"
)

func TestExecutionProfileReportIncludesSummaryAndBindings(t *testing.T) {
	profile := &executionProfile{
		started:         time.Now().Add(-25 * time.Millisecond),
		externByBinding: make(map[string]*bindingProfile),
	}

	profile.RecordDirectCall()
	profile.RecordModuleCall()
	profile.RecordClosureCreation(2)
	profile.RecordClosureCall(1)
	profile.RecordExternCall("DecodeInt", 1, 3*time.Millisecond)
	profile.RecordExternCall("JsonToDynamic", 1, 8*time.Millisecond)
	profile.RecordExternCall("DecodeInt", 1, 2*time.Millisecond)

	report := profile.Report()
	checks := []string{
		"[ard vm profile]",
		"calls direct=1 module=1 closure=1 extern=3",
		"closures created=1",
		"extern total=13ms",
		"JsonToDynamic calls=1 total=8ms",
		"DecodeInt calls=2 total=5ms",
	}
	for _, check := range checks {
		if !strings.Contains(report, check) {
			t.Fatalf("expected report to contain %q, got:\n%s", check, report)
		}
	}
}
