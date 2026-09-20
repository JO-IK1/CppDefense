package httpapi

import (
	"html/template"
	"io"
	"testing"

	appauth "github.com/JO-IK1/CppDefense/backend/internal/application/auth"
)

func TestAppTemplateRendersEveryRole(t *testing.T) {
	templates, err := template.ParseFS(webFiles, "templates/*.html")
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
