package tests

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegration(t *testing.T) {
	// 1. Build binaries
	wd, _ := os.Getwd()
	vouchPath := filepath.Join(wd, "vouch")
	cliPath := filepath.Join(wd, "vouch-cli")

	cmd := exec.Command("go", "build", "-o", vouchPath, "../main.go")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build vouch: %v", err)
	}
	defer os.Remove(vouchPath)

	cmd = exec.Command("go", "build", "-o", cliPath, "../cmd/vouch-cli/main.go")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build vouch-cli: %v", err)
	}
	defer os.Remove(cliPath)

	// 2. Setup environment
	tmpDir, _ := os.MkdirTemp("", "vouch-integration-*")
	defer os.RemoveAll(tmpDir)

	// Copy schema and policy
	schemaContent, _ := os.ReadFile("../schema.sql")
	_ = os.WriteFile(filepath.Join(tmpDir, "schema.sql"), schemaContent, 0644)

	policyContent := `
version: "2026.1"
policies:
  - id: critical-infra
    match_methods: ["aws:ec2:launch"]
    risk_level: critical
`
	_ = os.WriteFile(filepath.Join(tmpDir, "vouch-policy.yaml"), []byte(policyContent), 0644)

	// 3. Start Mock MCP Server
	mockServer := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var mcpReq map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&mcpReq)

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      mcpReq["id"],
				"result":  map[string]interface{}{"success": true},
			}
			json.NewEncoder(w).SetIndent("", "  ")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Printf("Mock server failed to encode response: %v", err)
			}
		}),
	}
	go func() {
		_ = mockServer.ListenAndServe()
	}()
	defer mockServer.Close()

	// 4. Start Vouch Proxy
	vouchCmd := exec.Command(vouchPath)
	vouchCmd.Dir = tmpDir
	// Create a pipe for stderr (standard log output goes here)
	stderr, _ := vouchCmd.StderrPipe()
	stdout, _ := vouchCmd.StdoutPipe()
	if err := vouchCmd.Start(); err != nil {
		t.Fatalf("Failed to start vouch: %v", err)
	}
	defer func() { _ = vouchCmd.Process.Kill() }()

	// Wait for readiness
	ready := make(chan bool)
	monitor := func(r io.Reader, name string) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Printf("[%s] %s\n", name, line)
			if strings.Contains(line, "Admin API:") {
				select {
				case ready <- true:
				default:
				}
			}
		}
	}
	go monitor(stdout, "vouch-out")
	go monitor(stderr, "vouch-err")

	select {
	case <-ready:
		// Proxy is ready
	case <-time.After(45 * time.Second): // Build and startup might take time
		t.Fatal("Timeout waiting for proxy readiness")
	}

	// 5. Test normal request (allow)
	req1 := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "mcp:list_tools",
		"params":  map[string]interface{}{},
	}
	resp1, err := sendRequest("http://localhost:9999", req1)
	if err != nil {
		t.Fatalf("Request 1 failed: %v", err)
	}
	if resp1["result"] == nil {
		t.Errorf("Expected result in response 1")
	}

	// 6. Test Forensic Capture (critical risk)
	req2 := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "aws:ec2:launch",
		"params":  map[string]interface{}{"type": "t2.micro"},
	}

	// In passive mode, this returns IMMEDIATELY (no stall)
	resp2, err := sendRequest("http://localhost:9999", req2)
	if err != nil {
		t.Fatalf("Request 2 failed: %v", err)
	}
	if resp2["result"] == nil {
		t.Errorf("Expected result in response 2 (passive mode should not block)")
	}

	// Wait for async ledger write
	time.Sleep(1 * time.Second)

	// 7. Verify recording and tagging via CLI
	cliRiskCmd := exec.Command(cliPath, "risk")
	cliRiskCmd.Dir = tmpDir
	out, _ := cliRiskCmd.CombinedOutput()
	fmt.Printf("[CLI RISK] %s\n", string(out))

	if !strings.Contains(strings.ToLower(string(out)), "critical") {
		t.Errorf("Expected 'critical' risk tag for aws:ec2:launch in CLI output")
	}

	// 8. Verify ledger integrity
	verifyCmd := exec.Command(cliPath, "verify", "--skip-live")
	verifyCmd.Dir = tmpDir
	out, err = verifyCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Verification failed: %v\nOutput: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Chain is valid") {
		t.Errorf("Verification output should contain 'Chain is valid', got:\n%s", string(out))
	}
}

func sendRequest(url string, reqBody interface{}) (map[string]interface{}, error) {
	b, _ := json.Marshal(reqBody)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var res map[string]interface{}
	_ = json.Unmarshal(body, &res)
	return res, nil
}
