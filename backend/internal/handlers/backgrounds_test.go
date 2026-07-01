package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mailgo/internal/authpassword"
	"mailgo/internal/middleware"

	"github.com/gorilla/mux"
)

func TestUploadBackgroundStoresMedia(t *testing.T) {
	t.Setenv("MAILGO_DATA_DIR", t.TempDir())

	body, contentType := multipartBody(t, "wallpaper.png", "image/png", []byte("\x89PNG\r\n\x1a\npng-body"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backgrounds", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	UploadBackground(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var res struct {
		URL        string `json:"url"`
		ContentTyp string `json:"content_type"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.URL, "/api/v1/backgrounds/serve/") {
		t.Fatalf("unexpected url %q", res.URL)
	}
	if res.ContentTyp != "image/png" {
		t.Fatalf("unexpected content type %q", res.ContentTyp)
	}

	name := strings.TrimPrefix(res.URL, "/api/v1/backgrounds/serve/")
	if _, err := os.Stat(filepath.Join(os.Getenv("MAILGO_DATA_DIR"), "backgrounds", name)); err != nil {
		t.Fatalf("stored file missing: %v", err)
	}
}

func TestUploadBackgroundRejectsHTML(t *testing.T) {
	t.Setenv("MAILGO_DATA_DIR", t.TempDir())

	body, contentType := multipartBody(t, "wallpaper.png", "image/png", []byte("<html>not an image</html>"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backgrounds", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	UploadBackground(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteBackgroundRemovesMedia(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MAILGO_DATA_DIR", dir)
	store := filepath.Join(dir, "backgrounds")
	if err := os.MkdirAll(store, 0755); err != nil {
		t.Fatal(err)
	}
	name := "0123456789abcdef0123456789abcdef.png"
	if err := os.WriteFile(filepath.Join(store, name), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/api/v1/backgrounds/{file}", DeleteBackground).Methods(http.MethodDelete)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/backgrounds/"+name, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(store, name)); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, stat err=%v", err)
	}
}

func TestBackgroundRoutesRequireToken(t *testing.T) {
	hash, err := authpassword.Hash("secret")
	if err != nil {
		t.Fatal(err)
	}
	auth := middleware.NewTokenAuth(hash)
	r := mux.NewRouter()
	api := r.PathPrefix("/api/v1").Subrouter()
	api.Use(auth.RequireToken)
	api.HandleFunc("/backgrounds", UploadBackground).Methods(http.MethodPost)
	api.HandleFunc("/backgrounds/serve/{file}", ServeBackground).Methods(http.MethodGet)
	api.HandleFunc("/backgrounds/{file}", DeleteBackground).Methods(http.MethodDelete)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/backgrounds"},
		{http.MethodGet, "/api/v1/backgrounds/serve/0123456789abcdef0123456789abcdef.png"},
		{http.MethodDelete, "/api/v1/backgrounds/0123456789abcdef0123456789abcdef.png"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func multipartBody(t *testing.T, filename, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}
