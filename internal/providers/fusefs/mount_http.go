package fusefs

import (
	"crypto/subtle"
	"io/fs"
	"net/http"
)

func projectionHandler(view fs.FS, token string) http.Handler {
	files := http.FileServer(http.FS(view))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Matrix-FS-Key")), []byte(token)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	})
}
