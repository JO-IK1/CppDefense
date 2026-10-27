package postgres

import (
	"strings"
	"testing"
	"time"
)

func TestReferenceResultValidation(t *testing.T) {
	valid := ReferenceResult{ProjectSHA256: strings.Repeat("a", 64), PolicySHA256: strings.Repeat("b", 64), ImageID: "sha256:" + strings.Repeat("c", 64), VerifiedAt: time.Now().UTC()}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"project", "policy", "image", "time", "future"} {
		value := valid
		switch field {
		case "project":
			value.ProjectSHA256 = "bad"
		case "policy":
			value.PolicySHA256 = ""
		case "image":
			value.ImageID = "mutable-tag"
		case "time":
			value.VerifiedAt = time.Time{}
		case "future":
			value.VerifiedAt = time.Now().Add(time.Hour)
		}
		if value.Validate() == nil {
			t.Fatalf("invalid proof accepted: %s", field)
		}
	}
}
