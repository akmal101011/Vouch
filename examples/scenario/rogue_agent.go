package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// In a real scenario, Vouch's interceptor would be running at :9999
// and the Target API would be at :8080.
const (
	VouchProxy = "http://localhost:9999"
)

func main() {
	client := &http.Client{Timeout: 5 * time.Second}

	fmt.Println("🤖 Rogue Agent starting task: 'Infrastructure Audit'")
	time.Sleep(1 * time.Second)

	// Step 1: List Instances
	fmt.Println("Step 1: Listing compute instances...")
	call(client, "GET", "/compute/instances", nil)

	time.Sleep(1 * time.Second)

	// Step 2: "Rogue" Action - Attempt to delete production database
	fmt.Println("Step 2: [CRITICAL] Attempting to decommission legacy database 'prod-users-v2'...")
	call(client, "POST", "/database/delete?name=prod-users-v2", nil)

	fmt.Println("\n🤖 Task finished (with security failure).")
	fmt.Println("🔍 Investigator: Use 'vouch-cli trace' to see what happened.")
}

func call(client *http.Client, method, path string, body interface{}) {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewBuffer(b)
	}

	req, err := http.NewRequest(method, VouchProxy+path, bodyReader)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Connection error: %v (Is Vouch running?)\n", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("   Response [%d]: %s\n", resp.StatusCode, string(respBody))
}
