package httpapi

import (
	"testing"
	"time"
)

func TestLocalImportTicketIsBoundToPlanAndExpires(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	plan := localImportPlan{Actor: "teacher", Expires: time.Now().Add(time.Hour).Unix(), Items: []localImportItem{{ID: "one", Repository: "org/lab-student", Branch: "latest"}}}
	ticket := signLocalPlan(plan, key)
	got, err := readLocalPlan(ticket, key)
	if err != nil || got.Items[0].Repository != plan.Items[0].Repository {
		t.Fatalf("roundtrip: %v %#v", err, got)
	}
	if _, err = readLocalPlan(ticket, []byte("other-key")); err == nil {
		t.Fatal("accepted another signing key")
	}
	if _, err = readLocalPlan("x"+ticket, key); err == nil {
		t.Fatal("accepted tampered ticket")
	}
	plan.Expires = time.Now().Add(-time.Second).Unix()
	if _, err = readLocalPlan(signLocalPlan(plan, key), key); err == nil {
		t.Fatal("accepted expired ticket")
	}
}
