package security

import "testing"

func TestTokenHashAndCSRF(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	hash := HashToken(key, token)
	if !VerifyToken(key, token, hash) {
		t.Fatal("token did not verify")
	}
	if VerifyToken(key, token+"x", hash) {
		t.Fatal("modified token verified")
	}

	csrf, csrfHash, err := NewCSRFToken(key, "session-id")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCSRFToken(key, "session-id", csrf, csrfHash); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCSRFToken(key, "other-session", csrf, csrfHash); err == nil {
		t.Fatal("cross-session csrf verified")
	}
}
