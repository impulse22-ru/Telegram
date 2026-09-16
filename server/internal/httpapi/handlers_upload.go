package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// handleUpload — POST /v1/upload. Загрузка изображения (multipart field "file").
// Возвращает {url: "/uploads/<name>"}. Размер ограничен 8 МБ.
// Имя файла — случайное hex-значение (криптографический rand) + ".jpg":
// исключает коллизии и обход пути через пользовательские имена.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "multipart required")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()

	// Создаём директорию загрузок при необходимости (она может отсутствовать).
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, "no storage")
		return
	}

	// 16 hex-символов (8 случайных байт) достаточно, чтобы избежать предсказуемых имён.
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		writeErr(w, http.StatusInternalServerError, "no entropy")
		return
	}
	name := hex.EncodeToString(b[:]) + ".jpg"

	dst, err := os.Create(filepath.Join(s.uploadDir, name))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "write error")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeErr(w, http.StatusInternalServerError, "copy error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"url": "/uploads/" + name})
}
