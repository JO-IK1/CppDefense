package importer

import (
	"archive/zip"
	"bytes"
	"testing"
)

func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestAnalyzeLabAndNormalizeProject(t *testing.T) {
	data := archive(t, map[string]string{
		ManifestName:             `{"schema_version":1,"kind":"lab","group_code":"iu7-21b","lab_code":"lab-01","student":{"github_login":"student-one"},"project_path":"project"}`,
		"project/CMakeLists.txt": "cmake_minimum_required(VERSION 3.20)",
		"project/main.cpp":       "int main() { return 0; }",
	})
	analysis, err := Analyze(data, "lab", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Projects) != 1 || len(analysis.Projects[0].Archive) == 0 {
		t.Fatalf("analysis = %#v", analysis)
	}
}

func TestAnalyzeAcceptsAndIgnoresLegacyDefenseSettings(t *testing.T) {
	data := archive(t, map[string]string{
		ManifestName:             `{"schema_version":1,"kind":"lab","group_code":"iu7-21b","lab_code":"lab-01","student":{"github_login":"student-one"},"project_path":"project","time_limit":600,"time_limit_seconds":900,"top_n":7}`,
		"project/CMakeLists.txt": "cmake_minimum_required(VERSION 3.20)",
		"project/main.cpp":       "int main() { return 0; }",
	})
	analysis, err := Analyze(data, "lab", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(analysis.Projects))
	}
}

func TestAnalyzeGroupAcceptsLegacyDefenseSettingsAtEveryOldLocation(t *testing.T) {
	data := archive(t, map[string]string{
		ManifestName:             `{"schema_version":1,"kind":"group","group_code":"iu7-21b","time_limit":600,"top_n":5,"items":[{"lab_code":"lab-01","student":{"github_login":"student-one"},"project_path":"project","time_limit_seconds":900,"top_n":7}]}`,
		"project/CMakeLists.txt": "cmake_minimum_required(VERSION 3.20)",
		"project/main.cpp":       "int main() { return 0; }",
	})
	analysis, err := Analyze(data, "group", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(analysis.Projects))
	}
}

func TestAnalyzeRejectsTraversal(t *testing.T) {
	data := archive(t, map[string]string{
		ManifestName:    `{"schema_version":1,"kind":"lab","group_code":"group","lab_code":"lab","student":{"github_login":"student"},"project_path":"project"}`,
		"../escape.cpp": "bad",
	})
	if _, err := Analyze(data, "lab", DefaultLimits()); err == nil {
		t.Fatal("ZIP Slip accepted")
	}
}

func TestAnalyzeRejectsProjectWithoutCMake(t *testing.T) {
	data := archive(t, map[string]string{
		ManifestName:       `{"schema_version":1,"kind":"lab","group_code":"group","lab_code":"lab","student":{"github_login":"student"},"project_path":"project"}`,
		"project/main.cpp": "int main() {}",
	})
	if _, err := Analyze(data, "lab", DefaultLimits()); err == nil {
		t.Fatal("project without CMake accepted")
	}
}
