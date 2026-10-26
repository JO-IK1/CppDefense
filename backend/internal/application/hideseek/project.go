// Package hideseek owns source-only mutations shared by the API and runner.
package hideseek

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

type Mutation struct {
	Path   string `json:"path"`
	Start  int    `json:"start"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// Editable is deliberately restrictive: tests, infrastructure and dependencies
// are never accepted as student edits, even if the client forges its request.
func Editable(value string) bool {
	if value == "" || value != path.Clean(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "../") || strings.ContainsAny(value, "\\\x00") {
		return false
	}
	parts := strings.Split(strings.ToLower(value), "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") || strings.Contains(part, "test") || strings.Contains(part, "spec") {
			return false
		}
		switch part {
		case "vendor", "third_party", "third-party", "external", "extern", "deps", "_deps", "build", "cmakefiles", "dist", "out", "cache", "node_modules", "cmake-build-debug", "cmake-build-release":
			return false
		}
	}
	switch strings.ToLower(path.Ext(value)) {
	case ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx":
		return true
	}
	return false
}

func Validate(mutations []Mutation, files map[string][]byte) error {
	if len(mutations) < 1 || len(mutations) > 5 {
		return fmt.Errorf("нужно от 1 до 5 изменённых мест")
	}
	ordered := append([]Mutation(nil), mutations...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Path == ordered[j].Path {
			return ordered[i].Start < ordered[j].Start
		}
		return ordered[i].Path < ordered[j].Path
	})
	var previous *Mutation
	for i := range ordered {
		m := &ordered[i]
		content, exists := files[m.Path]
		if !exists || !Editable(m.Path) {
			return fmt.Errorf("файл недоступен для изменения: %s", m.Path)
		}
		end := m.Start + len(m.Before)
		if m.Start < 0 || m.Start > len(content) || len(m.Before) == 0 || len(m.Before) > 16384 || len(m.After) > 16384 || end > len(content) || !utf8.ValidString(m.Before) || !utf8.ValidString(m.After) || m.Before == m.After || !bytes.Equal(content[m.Start:end], []byte(m.Before)) {
			return fmt.Errorf("неверный исходный фрагмент: %s", m.Path)
		}
		if previous != nil && previous.Path == m.Path && previous.Start+len(previous.Before) > m.Start {
			return fmt.Errorf("изменения пересекаются")
		}
		previous = m
	}
	return nil
}

func Apply(mutations []Mutation, files map[string][]byte) (map[string][]byte, error) {
	if err := Validate(mutations, files); err != nil {
		return nil, err
	}
	result := make(map[string][]byte, len(files))
	for name, data := range files {
		result[name] = bytes.Clone(data)
	}
	ordered := append([]Mutation(nil), mutations...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start > ordered[j].Start })
	for _, m := range ordered {
		data := result[m.Path]
		replacement := append([]byte{}, data[:m.Start]...)
		replacement = append(replacement, []byte(m.After)...)
		replacement = append(replacement, data[m.Start+len(m.Before):]...)
		result[m.Path] = replacement
	}
	return result, nil
}

func DecodeEdits(answer string, files map[string][]byte) (map[string]string, error) {
	var edits map[string]string
	if len(answer) > 1<<20 || json.Unmarshal([]byte(answer), &edits) != nil || edits == nil || len(edits) > 100 {
		return nil, fmt.Errorf("неверный формат правок проекта")
	}
	for name, content := range edits {
		if _, exists := files[name]; !exists || !Editable(name) || !utf8.ValidString(content) || len(content) > 1<<20 {
			return nil, fmt.Errorf("нельзя изменить файл: %s", name)
		}
	}
	return edits, nil
}
