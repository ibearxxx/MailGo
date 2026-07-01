package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
)

const backgroundMaxUploadBytes int64 = 50 * 1024 * 1024

var backgroundFileNamePattern = regexp.MustCompile(`^[a-f0-9]{32}\.(?:jpg|jpeg|png|gif|webp|avif|mp4|webm|ogg)$`)

type backgroundMediaType struct {
	contentType string
	extension   string
}

var allowedBackgroundMedia = map[string]backgroundMediaType{
	"image/jpeg": {contentType: "image/jpeg", extension: ".jpg"},
	"image/png":  {contentType: "image/png", extension: ".png"},
	"image/gif":  {contentType: "image/gif", extension: ".gif"},
	"image/webp": {contentType: "image/webp", extension: ".webp"},
	"image/avif": {contentType: "image/avif", extension: ".avif"},
	"video/mp4":  {contentType: "video/mp4", extension: ".mp4"},
	"video/webm": {contentType: "video/webm", extension: ".webm"},
	"video/ogg":  {contentType: "video/ogg", extension: ".ogg"},
}

// UploadBackground stores a user-provided background image/video on disk and
// returns a URL that can be saved in the appearance setting.
//
//	POST /api/v1/backgrounds
//	form field: file
func UploadBackground(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, backgroundMaxUploadBytes+1024*1024)
	if err := r.ParseMultipartForm(1024 * 1024); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid upload")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, backgroundMaxUploadBytes+1))
	if err != nil {
		respondError(w, http.StatusBadRequest, "Failed to read upload")
		return
	}
	if int64(len(data)) > backgroundMaxUploadBytes {
		respondError(w, http.StatusRequestEntityTooLarge, "Background media must be under 50 MB")
		return
	}
	if len(data) == 0 {
		respondError(w, http.StatusBadRequest, "Uploaded file is empty")
		return
	}

	media, ok := detectBackgroundMedia(data, header.Header.Get("Content-Type"), header.Filename)
	if !ok {
		respondError(w, http.StatusBadRequest, "Unsupported background media type")
		return
	}

	storeDir, err := backgroundStoreDir()
	if err != nil {
		log.Printf("background store dir: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to prepare background storage")
		return
	}
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		log.Printf("background store mkdir: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to prepare background storage")
		return
	}

	name, err := randomBackgroundFileName(media.extension)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to name background media")
		return
	}
	path := filepath.Join(storeDir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("background store write: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to save background media")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"url":          "/api/v1/backgrounds/serve/" + name,
		"content_type": media.contentType,
	})
}

// ServeBackground serves a previously-uploaded background image/video.
//
//	GET /api/v1/backgrounds/serve/{file}
func ServeBackground(w http.ResponseWriter, r *http.Request) {
	file := strings.TrimSpace(mux.Vars(r)["file"])
	if !isSafeBackgroundFileName(file) {
		http.NotFound(w, r)
		return
	}

	storeDir, err := backgroundStoreDir()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(storeDir, file)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeFile(w, r, path)
}

// DeleteBackground removes a previously-uploaded background file.
//
//	DELETE /api/v1/backgrounds/{file}
func DeleteBackground(w http.ResponseWriter, r *http.Request) {
	file := strings.TrimSpace(mux.Vars(r)["file"])
	if !isSafeBackgroundFileName(file) {
		http.NotFound(w, r)
		return
	}
	storeDir, err := backgroundStoreDir()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := os.Remove(filepath.Join(storeDir, file)); err != nil && !os.IsNotExist(err) {
		log.Printf("background delete: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to delete background media")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Background media deleted"})
}

func backgroundStoreDir() (string, error) {
	base, err := mailgoDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "backgrounds"), nil
}

func detectBackgroundMedia(data []byte, declaredType, filename string) (backgroundMediaType, bool) {
	declaredType = strings.ToLower(strings.TrimSpace(strings.Split(declaredType, ";")[0]))
	ext := strings.ToLower(filepath.Ext(filename))

	candidates := []string{declaredType}
	switch ext {
	case ".jpg", ".jpeg":
		candidates = append(candidates, "image/jpeg")
	case ".png":
		candidates = append(candidates, "image/png")
	case ".gif":
		candidates = append(candidates, "image/gif")
	case ".webp":
		candidates = append(candidates, "image/webp")
	case ".avif":
		candidates = append(candidates, "image/avif")
	case ".mp4":
		candidates = append(candidates, "video/mp4")
	case ".webm":
		candidates = append(candidates, "video/webm")
	case ".ogg", ".ogv":
		candidates = append(candidates, "video/ogg")
	}
	candidates = append(candidates, strings.ToLower(http.DetectContentType(data)))

	for _, c := range candidates {
		if media, ok := allowedBackgroundMedia[c]; ok && hasExpectedMediaSignature(data, c) {
			return media, true
		}
	}
	return backgroundMediaType{}, false
}

func hasExpectedMediaSignature(data []byte, contentType string) bool {
	switch contentType {
	case "image/jpeg":
		return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
	case "image/png":
		return len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n"
	case "image/gif":
		return len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a")
	case "image/webp":
		return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
	case "image/avif":
		return len(data) >= 12 && strings.Contains(string(data[4:12]), "ftyp")
	case "video/mp4":
		return len(data) >= 12 && strings.Contains(string(data[4:12]), "ftyp")
	case "video/webm":
		return len(data) >= 4 && data[0] == 0x1a && data[1] == 0x45 && data[2] == 0xdf && data[3] == 0xa3
	case "video/ogg":
		return len(data) >= 4 && string(data[:4]) == "OggS"
	default:
		return false
	}
}

func randomBackgroundFileName(ext string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]) + ext, nil
}

func isSafeBackgroundFileName(file string) bool {
	return backgroundFileNamePattern.MatchString(file)
}
