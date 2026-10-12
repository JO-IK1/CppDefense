package roster

import "testing"

func TestParseNormalizesRoster(t *testing.T) {
	document, err := Parse([]byte(`{"schema_version":1,"organization":{"github_login":"IS-ITMO-C-26","name":" ИС-26 "},"groups":[{"code":"26-013","name":"26-013","students":[{"full_name":" Иванов Иван ","github_login":"Ivanov"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if document.Organization.GitHubLogin != "is-itmo-c-26" || document.Groups[0].Students[0].GitHubLogin != "ivanov" || document.Groups[0].Students[0].FullName != "Иванов Иван" {
		t.Fatalf("unexpected normalized roster: %#v", document)
	}
}

func TestParseRejectsDuplicateStudentAcrossGroups(t *testing.T) {
	_, err := Parse([]byte(`{"schema_version":1,"organization":{"github_login":"is-itmo-c-26","name":"ИС-26"},"groups":[{"code":"26-013","name":"26-013","students":[{"full_name":"Иванов","github_login":"ivanov"}]},{"code":"26-014","name":"26-014","students":[{"full_name":"Иванов","github_login":"IVANOV"}]}]}`))
	if err == nil {
		t.Fatal("duplicate GitHub login accepted")
	}
}

func TestParseRejectsInvalidGitHubLogin(t *testing.T) {
	_, err := Parse([]byte(`{"schema_version":1,"organization":{"github_login":"is-itmo-c-26","name":"ИС-26"},"groups":[{"code":"26-013","name":"26-013","students":[{"full_name":"Иванов","github_login":"invalid--login"}]}]}`))
	if err == nil {
		t.Fatal("invalid GitHub login accepted")
	}
}
