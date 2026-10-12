package roster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var githubLoginPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
var codePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

type Document struct {
	SchemaVersion int          `json:"schema_version"`
	Organization  Organization `json:"organization"`
	Groups        []Group      `json:"groups"`
}

type Organization struct {
	GitHubLogin string `json:"github_login"`
	Name        string `json:"name"`
}

type Group struct {
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	Students []Student `json:"students"`
}

type Student struct {
	FullName    string `json:"full_name"`
	GitHubLogin string `json:"github_login"`
}

func Parse(data []byte) (Document, error) {
	var document Document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("JSON не соответствует формату: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Document{}, errors.New("после JSON-документа обнаружены лишние данные")
	}
	if document.SchemaVersion != 1 {
		return Document{}, errors.New("поддерживается только schema_version=1")
	}
	document.Organization.GitHubLogin = strings.ToLower(strings.TrimSpace(document.Organization.GitHubLogin))
	document.Organization.Name = strings.TrimSpace(document.Organization.Name)
	if !validGitHubLogin(document.Organization.GitHubLogin) || document.Organization.Name == "" {
		return Document{}, errors.New("у организации должны быть корректные github_login и name")
	}
	if len(document.Groups) == 0 || len(document.Groups) > 200 {
		return Document{}, errors.New("список groups должен содержать от 1 до 200 групп")
	}
	seenGroups := make(map[string]struct{}, len(document.Groups))
	seenStudents := make(map[string]string)
	for groupIndex := range document.Groups {
		group := &document.Groups[groupIndex]
		group.Code = strings.ToLower(strings.TrimSpace(group.Code))
		group.Name = strings.TrimSpace(group.Name)
		if !codePattern.MatchString(group.Code) || group.Name == "" {
			return Document{}, fmt.Errorf("группа %d содержит некорректные code или name", groupIndex+1)
		}
		if _, exists := seenGroups[group.Code]; exists {
			return Document{}, fmt.Errorf("группа %q указана повторно", group.Code)
		}
		seenGroups[group.Code] = struct{}{}
		if len(group.Students) > 10_000 {
			return Document{}, fmt.Errorf("в группе %q слишком много студентов", group.Code)
		}
		for studentIndex := range group.Students {
			student := &group.Students[studentIndex]
			student.FullName = strings.TrimSpace(student.FullName)
			student.GitHubLogin = strings.ToLower(strings.TrimSpace(student.GitHubLogin))
			if student.FullName == "" || len([]rune(student.FullName)) > 200 || !validGitHubLogin(student.GitHubLogin) {
				return Document{}, fmt.Errorf("студент %d в группе %q содержит некорректные full_name или github_login", studentIndex+1, group.Code)
			}
			if previous, exists := seenStudents[student.GitHubLogin]; exists {
				return Document{}, fmt.Errorf("GitHub-логин %q повторяется в группах %q и %q", student.GitHubLogin, previous, group.Code)
			}
			seenStudents[student.GitHubLogin] = group.Code
		}
	}
	return document, nil
}

func validGitHubLogin(value string) bool {
	return githubLoginPattern.MatchString(value) && !strings.Contains(value, "--")
}
