package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type Instance struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

var instances = []Instance{
	{ID: "i-0123456", Status: "running", Type: "t3.medium"},
	{ID: "i-0abcdef", Status: "running", Type: "m5.large"},
}

func main() {
	mux := http.NewServeMux()

	// List Instances (Low Risk)
	mux.HandleFunc("/compute/instances", func(w http.ResponseWriter, r *http.Request) {
		log.Println("GET /compute/instances")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(instances)
	})

	// Delete Database (High Risk Scenario)
	mux.HandleFunc("/database/delete", func(w http.ResponseWriter, r *http.Request) {
		dbName := r.URL.Query().Get("name")
		log.Printf("DELETE /database/delete?name=%s", dbName)

		if dbName == "prod-users-v2" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"error": "Unauthorized: Production database deletion requires MFA"}`)
			return
		}

		fmt.Fprintf(w, `{"status": "deleted", "database": "%s"}`, dbName)
	})

	log.Println("Mock Cloud API started on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
