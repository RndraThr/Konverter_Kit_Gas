package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestLoginPageRendersBrandingAndCarouselAssets(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	response := httptest.NewRecorder()

	newTestHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	body := response.Body.String()
	expectedSnippets := []string{
		"Sistem Manajemen Program Konkit Gas",
		`id="konkit-root"`,
		"/static/app/assets/",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected login page to contain %q", snippet)
		}
	}

	if strings.Contains(body, "Super Admin") {
		t.Fatal("expected login page not to show the Super Admin label")
	}
}

func TestLoginPageServesReactShell(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	response := httptest.NewRecorder()

	newTestHandler().ServeHTTP(response, request)

	body := response.Body.String()
	if !strings.Contains(body, `id="konkit-root"`) {
		t.Fatal("expected React mount element")
	}
	if !strings.Contains(body, `/static/app/assets/`) {
		t.Fatal("expected Vite built asset path")
	}
}

func TestRootRedirectsToLogin(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	newTestHandler().ServeHTTP(response, request)

	if response.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", response.Code)
	}

	location := response.Header().Get("Location")
	if location != "/login" {
		t.Fatalf("expected redirect to /login, got %q", location)
	}
}

func TestFaviconIsServed(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	response := httptest.NewRecorder()

	newTestHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
}

func TestLoginCarouselServesAcehPhotos(t *testing.T) {
	handler := newTestHandler()
	assetPaths := []string{
		"/static/images/konkit-aceh-socialization.jpg",
		"/static/images/konkit-aceh-demonstration.jpg",
		"/static/images/konkit-aceh-recipient.jpg",
	}

	for _, assetPath := range assetPaths {
		request := httptest.NewRequest(http.MethodGet, assetPath, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("expected carousel asset %s to return 200, got %d", assetPath, response.Code)
		}
	}

	pageRequest := httptest.NewRequest(http.MethodGet, "/login", nil)
	pageResponse := httptest.NewRecorder()
	handler.ServeHTTP(pageResponse, pageRequest)

	scriptMatch := regexp.MustCompile(`src="([^"]+\.js)"`).FindStringSubmatch(pageResponse.Body.String())
	if len(scriptMatch) != 2 {
		t.Fatal("expected login page to include a JavaScript bundle")
	}

	scriptRequest := httptest.NewRequest(http.MethodGet, scriptMatch[1], nil)
	scriptResponse := httptest.NewRecorder()
	handler.ServeHTTP(scriptResponse, scriptRequest)
	if scriptResponse.Code != http.StatusOK {
		t.Fatalf("expected login JavaScript bundle to return 200, got %d", scriptResponse.Code)
	}

	bundle := scriptResponse.Body.String()
	for _, assetPath := range assetPaths {
		if !strings.Contains(bundle, assetPath) {
			t.Fatalf("expected login bundle to reference carousel asset %s", assetPath)
		}
	}
}
