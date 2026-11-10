package postgres

import "testing"

func TestMixedRestoreSelectionAndOverlaps(t *testing.T) {
	catalog := []WheelCandidate{
		{ID: "box", EntityType: "struct", FilePath: "a.cpp", SignatureBegin: 0, BodyEnd: 100},
		{ID: "method", EntityType: "function", FilePath: "a.cpp", SignatureBegin: 20, BodyEnd: 50},
		{ID: "f", EntityType: "function", FilePath: "a.cpp", SignatureBegin: 101, BodyEnd: 120},
		{ID: "g", EntityType: "function", FilePath: "b.cpp", SignatureBegin: 0, BodyEnd: 30},
	}
	_, targets, ok := selectRestoreTargets(catalog, "manual", "", "42", 3, RestoreOptions{HiddenCount: 3, CandidateIDs: []string{"box", "f", "g"}})
	if !ok || len(targets) != 3 || targets[0].ID != "box" {
		t.Fatal("mixed selection rejected", targets)
	}
	for _, ids := range [][]string{{"box", "method"}, {"f", "f"}, {"f", "missing"}} {
		if _, _, ok := selectRestoreTargets(catalog, "manual", "", "42", 2, RestoreOptions{HiddenCount: 2, CandidateIDs: ids}); ok {
			t.Fatal("invalid selection accepted", ids)
		}
	}
	catalog[3].IsTestFile = true
	if _, _, ok := selectRestoreTargets(catalog, "automatic", "", "42", 4, RestoreOptions{HiddenCount: 2}); ok {
		t.Fatal("automatic mode included a test")
	}
	catalog[3].IsTestFile = false
	_, targets, ok = selectRestoreTargets(catalog, "automatic", "", "42", 4, RestoreOptions{HiddenCount: 3})
	if !ok || len(targets) != 3 {
		t.Fatal("non-overlapping automatic selection failed")
	}
	for i, a := range targets {
		for _, b := range targets[i+1:] {
			if !nonOverlapping(a, b) {
				t.Fatal("overlap", a, b)
			}
		}
	}
}
