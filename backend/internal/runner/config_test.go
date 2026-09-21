package runner

import "testing"

func TestLoadRejectsTooManySlots(t *testing.T) {
	t.Setenv("CPPDEFENSE_BACKEND_URL", "http://backend")
	t.Setenv("CPPDEFENSE_RUNNER_TOKEN", "01234567890123456789012345678901")
	t.Setenv("CPPDEFENSE_RUNNER_ID", "0199a123-4567-7abc-8def-0123456789ab")
	t.Setenv("CPPDEFENSE_RUNNER_SLOTS", "7")
	if _, err := load(); err == nil {
		t.Fatal("invalid slot count accepted")
	}
}
