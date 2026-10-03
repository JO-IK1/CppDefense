package httpapi

import (
	"bytes"
	"html/template"
	"io"
	"strings"
	"testing"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
	"github.com/JO-IK1/CppDefense/backend/webassets"
)

func TestAppTemplateRendersEveryRole(t *testing.T) {
	templates, err := template.ParseFS(webassets.Files, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"student", "teacher", "admin"} {
		value := role
		page := appPage{User: appauth.User{Status: "active", Role: &value}, Role: role}
		if err := templates.ExecuteTemplate(io.Discard, "app.html", page); err != nil {
			t.Fatalf("role %s: %v", role, err)
		}
	}
}

func TestStudentWorkspaceIncludesAccessibleResizer(t *testing.T) {
	templates, err := template.ParseFS(webassets.Files, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	role := "student"
	page := appPage{User: appauth.User{Status: "active", Role: &role}, Role: role}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, "app.html", page); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`id="workspace-resizer"`, `role="separator"`, `aria-valuenow="33"`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("student workspace is missing %s", expected)
		}
	}
}
