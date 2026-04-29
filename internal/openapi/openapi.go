package openapi

import (
	"embed"
	"fmt"
	"net/http"
	"strings"
)

// We embed the OpenAPI YAML specs and Swagger UI HTML into the binary so the
// docs work in the scratch container image.

//go:embed spec/public.yaml spec/admin.yaml swaggerui/public.html swaggerui/admin.html
var assets embed.FS

func SpecPublicYAML() ([]byte, error) {
	fmt.Println("spec/public.yaml requested")
	return assets.ReadFile("spec/public.yaml")
}

func SpecAdminYAML() ([]byte, error) {
	return assets.ReadFile("spec/admin.yaml")
}

func SwaggerUIHTMLPublic() ([]byte, error) {
	fmt.Println("swaggerui/public.html requested")
	return assets.ReadFile("swaggerui/public.html")
}

func SwaggerUIHTMLAdmin() ([]byte, error) {
	return assets.ReadFile("swaggerui/admin.html")
}

func WriteYAML(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(b)
}

func WriteHTML(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

// InjectSpecURL replaces the placeholder token in the embedded HTML.
func InjectSpecURL(html []byte, specURL string) ([]byte, error) {
	if specURL == "" {
		return nil, fmt.Errorf("specURL is required")
	}
	out := strings.ReplaceAll(string(html), "__SPEC_URL__", specURL)
	return []byte(out), nil
}
