package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

// Decode exactly one string for each opaque target ID; duplicate/unknown IDs fail closed.
func restoreAnswers(lease postgres.Lease) (map[string]string, error) {
	if lease.Answer == nil {
		return nil, fmt.Errorf("missing answer")
	}
	decoder := json.NewDecoder(strings.NewReader(*lease.Answer))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, fmt.Errorf("answers must be an object")
	}
	allowed := map[string]bool{}
	for _, t := range lease.RestoreTargets {
		allowed[t.ID] = true
	}
	answers := map[string]string{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		id, ok := token.(string)
		if !ok || !allowed[id] {
			return nil, fmt.Errorf("unknown target")
		}
		if _, found := answers[id]; found {
			return nil, fmt.Errorf("duplicate target")
		}
		var answer string
		if err = decoder.Decode(&answer); err != nil {
			return nil, err
		}
		answers[id] = answer
	}
	if _, err = decoder.Token(); err != nil {
		return nil, err
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing answer data")
	}
	if len(answers) != len(allowed) {
		return nil, fmt.Errorf("answer every selected element")
	}
	return answers, nil
}

func restorePayload(lease postgres.Lease) (map[string]any, error) {
	payload := map[string]any{"project_root": "project", "output_root": "attempt", "selected_function": lease.SelectedFunction}
	if len(lease.RestoreTargets) == 0 {
		if lease.Answer == nil {
			return nil, fmt.Errorf("missing answer")
		}
		payload["answer"] = *lease.Answer
		return payload, nil
	}
	answers, err := restoreAnswers(lease)
	if err != nil {
		return nil, err
	}
	replacements := make([]map[string]any, 0)
	for _, t := range lease.RestoreTargets {
		replacements = append(replacements, map[string]any{"selected_function": t.SelectedFunction, "answer": answers[t.ID], "full_definition": !lease.SignaturesVisible})
	}
	for key, value := range replacements[0] {
		payload[key] = value
	}
	if len(replacements) > 1 {
		payload["additional_replacements"] = replacements[1:]
	}
	return payload, nil
}
