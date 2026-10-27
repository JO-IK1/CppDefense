package runner

import (
	"context"
	"sync"
	"time"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

type timingKey struct{}
type jobProfile struct {
	sync.Mutex
	started time.Time
	value   postgres.JobTimings
}

func profile(ctx context.Context) *jobProfile {
	value, _ := ctx.Value(timingKey{}).(*jobProfile)
	return value
}
func addTiming(ctx context.Context, apply func(*postgres.JobTimings)) {
	if p := profile(ctx); p != nil {
		p.Lock()
		apply(&p.value)
		p.Unlock()
	}
}
func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}
func timingPointer(v float64) *float64 { return &v }
