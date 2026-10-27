package postgres

import (
	"fmt"
	"math"
)

// Milliseconds; compile/link are summed command wall times, and can overlap.
// These are diagnostics, never evidence of correctness or resource enforcement.
type JobTimings struct {
	QueueMS         float64  `json:"queue_ms"`
	DownloadMS      float64  `json:"download_ms"`
	MaterializeMS   float64  `json:"materialize_ms"`
	ContainerMS     float64  `json:"container_ms"`
	ConfigureMS     float64  `json:"configure_ms"`
	CompileMS       *float64 `json:"compile_ms"`
	LinkMS          *float64 `json:"link_ms"`
	BuildMS         float64  `json:"build_ms"`
	TestMS          float64  `json:"test_ms"`
	SnapshotMS      float64  `json:"snapshot_ms"`
	TotalMS         float64  `json:"total_ms"`
	SnapshotHit     bool     `json:"snapshot_hit"`
	CompileCommands int      `json:"compile_commands"`
	LinkCommands    int      `json:"link_commands"`
}

func (t JobTimings) Validate() error {
	for _, v := range []float64{t.QueueMS, t.TotalMS} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("invalid queue/total timing")
		}
	}
	values := []float64{t.DownloadMS, t.MaterializeMS, t.ContainerMS, t.ConfigureMS, t.BuildMS, t.TestMS, t.SnapshotMS}
	if t.CompileMS != nil {
		values = append(values, *t.CompileMS)
	}
	if t.LinkMS != nil {
		values = append(values, *t.LinkMS)
	}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 7*24*3600*1000 {
			return fmt.Errorf("invalid job timing")
		}
	}
	if t.CompileCommands < 0 || t.LinkCommands < 0 || t.CompileCommands > 1000000 || t.LinkCommands > 1000000 {
		return fmt.Errorf("invalid command count")
	}
	return nil
}
