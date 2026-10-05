package postgres

import "testing"

func candidate(id string, lines int, testFile bool) WheelCandidate {
	return WheelCandidate{ID: id, FunctionName: id, EntityType: "function", LineCount: lines, IsTestFile: testFile}
}

func TestAutomaticWheelExcludesTestFiles(t *testing.T) {
	catalog := []WheelCandidate{
		candidate("largest", 100, false),
		candidate("second", 80, false),
		candidate("random-a", 20, false),
		candidate("random-b", 10, false),
		candidate("test-helper", 1000, true),
	}
	wheel, selected, ok := buildWheel(catalog, "automatic", "", "42", 3)
	if !ok || len(wheel) != 3 || selected.ID == "" {
		t.Fatalf("unexpected automatic wheel: ok=%v wheel=%+v selected=%+v", ok, wheel, selected)
	}
	for _, value := range wheel {
		if value.IsTestFile {
			t.Fatalf("test-file candidate entered automatic wheel: %+v", value)
		}
	}
}

func TestManualWheelAllowsExplicitTestCandidate(t *testing.T) {
	catalog := []WheelCandidate{
		candidate("normal-a", 100, false),
		candidate("normal-b", 80, false),
		candidate("normal-c", 20, false),
		candidate("test-target", 5, true),
	}
	wheel, selected, ok := buildWheel(catalog, "manual", "test-target", "7", 3)
	if !ok || selected.ID != "test-target" || len(wheel) != 3 {
		t.Fatalf("unexpected manual wheel: ok=%v wheel=%+v selected=%+v", ok, wheel, selected)
	}
	seen := false
	for _, value := range wheel {
		if value.ID == "test-target" {
			seen = true
		}
		if value.IsTestFile && value.ID != "test-target" {
			t.Fatalf("unselected test-file candidate entered manual wheel: %+v", value)
		}
	}
	if !seen {
		t.Fatal("manual target is missing from wheel")
	}
}

func TestWheelRequiresRequestedCandidateCount(t *testing.T) {
	catalog := []WheelCandidate{candidate("one", 1, false)}
	if _, _, ok := buildWheel(catalog, "automatic", "", "1", 2); ok {
		t.Fatal("wheel unexpectedly accepted an undersized catalog")
	}
}

func TestAutomaticWheelUsesTwoThirdsLargestAndOneThirdRandom(t *testing.T) {
	catalog := []WheelCandidate{
		candidate("largest-1", 100, false),
		candidate("largest-2", 90, false),
		candidate("largest-3", 80, false),
		candidate("largest-4", 70, false),
		candidate("random-pool-1", 60, false),
		candidate("random-pool-2", 50, false),
		candidate("random-pool-3", 40, false),
		candidate("random-pool-4", 30, false),
	}
	wheel, _, ok := buildWheel(catalog, "automatic", "", "fixed-seed", 6)
	if !ok {
		t.Fatal("automatic wheel was rejected")
	}
	ids := make(map[string]bool, len(wheel))
	for _, value := range wheel {
		ids[value.ID] = true
	}
	for _, id := range []string{"largest-1", "largest-2", "largest-3", "largest-4"} {
		if !ids[id] {
			t.Fatalf("required largest candidate %q is missing: %+v", id, wheel)
		}
	}
	if len(wheel) != 6 {
		t.Fatalf("wheel size = %d, want 6", len(wheel))
	}
}

func TestPreparationStageDistinguishesBothRunnerPhases(t *testing.T) {
	selected := "candidate"
	tests := []struct {
		name    string
		defense Defense
		want    string
	}{
		{name: "catalog", defense: Defense{Status: "preparing"}, want: "analyzing_project"},
		{name: "teacher", defense: Defense{Status: "ready"}, want: "waiting_for_teacher"},
		{name: "challenge", defense: Defense{Status: "preparing", SelectedCandidateID: &selected}, want: "materializing_challenge"},
		{name: "active", defense: Defense{Status: "active", SelectedCandidateID: &selected}, want: "ready"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := preparationStage(test.defense); got != test.want {
				t.Fatalf("preparationStage() = %q, want %q", got, test.want)
			}
		})
	}
}
