package postgres

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefenseAlwaysSerializesWheelCandidates(t *testing.T) {
	value := Defense{WheelCandidates: make([]WheelCandidate, 0)}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"wheel_candidates":[]`) {
		t.Fatalf("wheel candidates must be an empty array while preparation is pending: %s", encoded)
	}
}
