package hideseek

import "testing"

func TestMutationsUseOriginalOffsetsAndRejectOverlap(t *testing.T) {
	files := map[string][]byte{"src/main.cpp": []byte("a > b && c < d")}
	mutations := []Mutation{{Path: "src/main.cpp", Start: 2, Before: ">", After: ">="}, {Path: "src/main.cpp", Start: 6, Before: "&", After: "|"}}
	changed, err := Apply(mutations, files)
	if err != nil {
		t.Fatal(err)
	}
	if string(changed["src/main.cpp"]) != "a >= b |& c < d" {
		t.Fatalf("got %q", changed["src/main.cpp"])
	}
	if string(files["src/main.cpp"]) != "a > b && c < d" {
		t.Fatal("original changed")
	}
	mutations[1] = Mutation{Path: "src/main.cpp", Start: 2, Before: "> b", After: "x"}
	if Validate(mutations, files) == nil {
		t.Fatal("overlap accepted")
	}
}

func TestProtectedFilesCannotBeMutatedOrSubmitted(t *testing.T) {
	for _, name := range []string{"tests/main.cpp", "src/foo_test.hpp", "CMakeLists.txt", "vendor/lib/a.cpp", "../a.cpp", ".git/source.c", "build/main.cpp"} {
		files := map[string][]byte{name: []byte("a")}
		if Validate([]Mutation{{Path: name, Before: "a", After: "b"}}, files) == nil {
			t.Errorf("mutation accepted: %s", name)
		}
		if _, err := DecodeEdits(`{"`+name+`":"b"}`, files); err == nil {
			t.Errorf("edit accepted: %s", name)
		}
	}
	if _, err := DecodeEdits(`{"missing.cpp":"x"}`, map[string][]byte{"main.cpp": []byte("a")}); err == nil {
		t.Fatal("new file accepted")
	}
}

func TestMutationCountAndDeletion(t *testing.T) {
	files := map[string][]byte{"main.cpp": []byte("abcdef")}
	if Validate(nil, files) == nil {
		t.Fatal("zero mutations accepted")
	}
	var mutations []Mutation
	for index := 0; index < 6; index++ {
		mutations = append(mutations, Mutation{Path: "main.cpp", Start: index, Before: string(rune('a' + index)), After: ""})
	}
	if Validate(mutations, files) == nil {
		t.Fatal("six mutations accepted")
	}
	changed, err := Apply(mutations[:5], files)
	if err != nil {
		t.Fatal(err)
	}
	if string(changed["main.cpp"]) != "f" {
		t.Fatalf("deletion: %s", changed["main.cpp"])
	}
}
