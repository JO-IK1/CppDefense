package postgres

import (
	"encoding/hex"
	"fmt"
	"time"
)

// ReferenceResult is supplied by the authenticated runner, never by a student.
// It identifies a previously tested original, not a newly executed test run.
type ReferenceResult struct {
	ProjectSHA256 string    `json:"project_sha256"`
	ImageID       string    `json:"image_id"`
	PolicySHA256  string    `json:"policy_sha256"`
	VerifiedAt    time.Time `json:"verified_at"`
}

func (v ReferenceResult) Validate() error {
	for _, value := range []string{v.ProjectSHA256, v.PolicySHA256} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("invalid reference digest")
		}
	}
	if len(v.ImageID) != 71 || v.ImageID[:7] != "sha256:" {
		return fmt.Errorf("invalid reference image")
	}
	decoded, err := hex.DecodeString(v.ImageID[7:])
	if err != nil || len(decoded) != 32 || v.VerifiedAt.IsZero() || v.VerifiedAt.After(time.Now().Add(time.Minute)) {
		return fmt.Errorf("invalid reference proof")
	}
	return nil
}
