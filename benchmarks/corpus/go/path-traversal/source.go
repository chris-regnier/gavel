package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func handleDownload(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("file")
	path := filepath.Join("/var/data/uploads", filename)

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	io.Copy(w, f)
}
