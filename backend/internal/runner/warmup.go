package runner

import (
	"context"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func (a *agent) warmSubmission(parent, ctx context.Context, project string, lease postgres.Lease) error {
	build, result := a.prepareBuildContext(ctx, project, lease)
	outcome := "error"
	if build != nil && result != nil {
		outcome = "failed"
		if result.OK() {
			outcome = "passed"
		} else if result.InfrastructureError {
			outcome = "error"
		}
	}
	// A warmup never creates a defense, starts its timer or awards a grade.
	return a.complete(parent, lease, postgres.Completion{Outcome: outcome})
}
