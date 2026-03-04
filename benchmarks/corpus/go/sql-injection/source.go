package main

import (
	"database/sql"
	"fmt"
	"net/http"
)

func handleSearch(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	username := r.URL.Query().Get("username")
	query := fmt.Sprintf("SELECT * FROM users WHERE username = '%s'", username)
	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	fmt.Fprintf(w, "Results found")
}
