package httpapi

import (
	"strings"
	"testing"
)

func TestMaskRepositoryBodyHidesOnlySelectedImplementation(t *testing.T) {
	source := []byte("int kept() { return 1; }\nint hidden() {\n  return 42;\n}\n")
	hidden := strings.Index(string(source), "int hidden()")
	begin := int64(hidden + strings.Index(string(source[hidden:]), "{"))
	end := int64(strings.Index(string(source[begin:]), "}")) + begin + 1
	masked, err := maskRepositoryBody(source, begin, end)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(masked), "return 42") {
		t.Fatal("selected implementation leaked")
	}
	if !strings.Contains(string(masked), "return 1") || !strings.Contains(string(masked), "/* TODO */") {
		t.Fatalf("unexpected masked source: %s", masked)
	}
}

func TestSafeRepositoryPath(t *testing.T) {
	for _, value := range []string{"../secret", "/absolute", "a/../../b", `a\\b`} {
		if safeRepositoryPath(value) {
			t.Fatalf("unsafe path accepted: %q", value)
		}
	}
	if !safeRepositoryPath("src/main.cpp") {
		t.Fatal("safe repository path rejected")
	}
}
