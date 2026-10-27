package postgres

import (
	"math"
	"testing"
)

func TestJobTimingsValidation(t *testing.T) {
	if err := (JobTimings{}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{-1, math.NaN(), math.Inf(1), 8 * 24 * 3600 * 1000} {
		if (JobTimings{ContainerMS: v}).Validate() == nil {
			t.Fatalf("invalid timing accepted: %v", v)
		}
		if (JobTimings{QueueMS: 8 * 24 * 3600 * 1000, TotalMS: 8 * 24 * 3600 * 1000}).Validate() != nil {
			t.Fatal("legitimate old queued job rejected")
		}
		if (JobTimings{CompileMS: &v}).Validate() == nil {
			t.Fatalf("invalid command timing accepted: %v", v)
		}
	}
	if (JobTimings{CompileCommands: -1}).Validate() == nil {
		t.Fatal("negative command count")
	}
}
