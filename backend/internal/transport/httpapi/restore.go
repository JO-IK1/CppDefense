package httpapi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

// Preserve newlines, not byte width: markers must fit even a one-character body.
func maskRestoreTargets(source []byte, file string, targets []postgres.RestoreTarget, visible bool) ([]byte, error) {
	selected := make([]postgres.RestoreTarget, 0)
	for _, t := range targets {
		if t.FilePath == file {
			selected = append(selected, t)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].SignatureBegin > selected[j].SignatureBegin })
	out := string(source)
	boundary := int64(len(source))
	for _, t := range selected {
		begin, end := t.BodyBegin+1, t.BodyEnd-1
		if !visible {
			begin = t.SignatureBegin
			end = t.BodyEnd
			if end < int64(len(source)) && source[end] == ';' {
				end++
			}
		}
		if begin < 0 || end < begin || end > int64(len(source)) || end > boundary {
			return nil, fmt.Errorf("invalid restore ranges")
		}
		original := string(source[begin:end])
		replacement := "/* " + t.Label + " */" + strings.Repeat("\n", strings.Count(original, "\n"))
		out = out[:begin] + replacement + out[end:]
		boundary = begin
	}
	return []byte(out), nil
}
