package httpapi

import (
	"strings"
	"testing"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

func TestRestoreMaskWholeDefinitionsAndTinyBodies(t *testing.T) {
	source := "int f(){1}\nstruct Box{int x;};\nint g(){2}\n"
	targets := []postgres.RestoreTarget{
		{Label: "func_1", SelectedFunction: postgres.SelectedFunction{FilePath: "a.cpp", SignatureBegin: 0, BodyBegin: 7, BodyEnd: 10}},
		{Label: "func_2", SelectedFunction: postgres.SelectedFunction{FilePath: "a.cpp", SignatureBegin: 11, BodyBegin: 21, BodyEnd: 29}},
	}
	for _, visible := range []bool{true, false} {
		value, err := maskRestoreTargets([]byte(source), "a.cpp", targets, visible)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(value), "func_1") || !strings.Contains(string(value), "func_2") || !strings.Contains(string(value), "int g(){2}") {
			t.Fatal(string(value))
		}
		if strings.Count(string(value), "\n") != strings.Count(source, "\n") {
			t.Fatal("line numbers changed")
		}
		if !visible && (strings.Contains(string(value), "int f") || strings.Contains(string(value), "struct Box")) {
			t.Fatal("signature leaked", string(value))
		}
	}
	value, err := maskRestoreTargets([]byte(source), "header.hpp", targets, false)
	if err != nil || string(value) != source {
		t.Fatal("unselected declarations changed")
	}
}
