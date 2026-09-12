package domain

import "testing"

func TestImportTransitions(t *testing.T) {
	if !CanTransitionImport(ImportReceiving, ImportStored) {
		t.Fatal("valid transition rejected")
	}
	if CanTransitionImport(ImportCompleted, ImportApplying) {
		t.Fatal("terminal transition accepted")
	}
	if CanTransitionImport(ImportReviewPending, ImportCompleted) {
		t.Fatal("review bypass accepted")
	}
}

func TestUUIDv7(t *testing.T) {
	id, err := NewUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 || id[14] != '7' {
		t.Fatalf("invalid UUIDv7 %q", id)
	}
}
