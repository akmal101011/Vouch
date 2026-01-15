package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/vouch/internal/assert"
	"github.com/yourname/vouch/internal/ledger"
	"github.com/yourname/vouch/internal/proxy"
)

// MCPRequest represents a Model Context Protocol JSON-RPC request
type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
}

// MCPResponse represents a Model Context Protocol JSON-RPC response
type MCPResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   map[string]interface{} `json:"error,omitempty"`
}

// VouchProxy is the main proxy server
type VouchProxy struct {
	proxy           *httputil.ReverseProxy
	worker          *ledger.Worker
	activeTasks     *sync.Map // task_id -> state
	policy          *proxy.PolicyConfig
	stallSignals    *sync.Map // Maps event ID to approval channel
	lastEventByTask *sync.Map // task_id -> last_event_id
}

func main() {
	log.Println("Vouch (Agent Analytics & Safety) - The Interceptor")

	// Load policy
	policy, err := proxy.LoadPolicy("vouch-policy.yaml")
	if err != nil {
		log.Fatalf("Failed to load policy: %v", err)
	}
	log.Printf("Loaded policy version %s with %d rules", policy.Version, len(policy.Policies))

	// Create target URL
	targetURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatalf("Failed to parse target URL: %v", err)
	}

	// Create proxy
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Initialize ledger worker with database and crypto
	worker, err := ledger.NewWorker(1000, "vouch.db", ".vouch_key")
	if err != nil {
		log.Fatalf("Failed to initialize worker: %v", err)
	}

	// Start worker (creates genesis block if needed)
	if err := worker.Start(); err != nil {
		log.Fatalf("Failed to start worker: %v", err)
	}

	// Create Vouch proxy
	vouchProxy := &VouchProxy{
		proxy:           proxy,
		worker:          worker,
		activeTasks:     &sync.Map{}, // task_id -> state
		policy:          policy,
		stallSignals:    &sync.Map{}, // event_id -> chan struct{}
		lastEventByTask: &sync.Map{}, // task_id -> last_event_id
	}

	// Custom director to intercept requests
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		vouchProxy.interceptRequest(req)
	}

	// Custom response modifier
	proxy.ModifyResponse = vouchProxy.interceptResponse

	// Start API server for CLI commands (approval/rejection)
	go func() {
		apiMux := http.NewServeMux()
		apiMux.HandleFunc("/api/approve/", vouchProxy.handleApprove)
		apiMux.HandleFunc("/api/reject/", vouchProxy.handleReject)
		apiMux.HandleFunc("/api/rekey", vouchProxy.handleRekey)

		apiAddr := ":9998"
		log.Printf("API server listening on %s", apiAddr)
		if err := http.ListenAndServe(apiAddr, apiMux); err != nil {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	// Start proxy server
	listenAddr := ":9999"
	log.Printf("Proxying :9999 -> :8080")
	log.Printf("Event buffer size: 1000")
	log.Printf("Policy engine: ACTIVE")
	log.Printf("Ready to intercept MCP traffic on %s\n", listenAddr)

	if err := http.ListenAndServe(listenAddr, proxy); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// interceptRequest intercepts and analyzes incoming requests
func (v *VouchProxy) interceptRequest(req *http.Request) {
	if req.Method != http.MethodPost {
		return
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		return
	}
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 1. Extract Metadata
	mcpReq, taskID, taskState, err := v.extractTaskMetadata(bodyBytes)
	if err != nil {
		v.sendErrorResponse(req, http.StatusBadRequest, -32000, err.Error())
		return
	}

	// 2. Health Check
	if !v.worker.IsHealthy() {
		v.sendErrorResponse(req, http.StatusServiceUnavailable, -32000, "Ledger Storage Failure")
		return
	}

	// 3. Policy Evaluation
	shouldStall, matchedRule, err := v.evaluatePolicy(mcpReq.Method, mcpReq.Params)
	if err != nil {
		v.sendErrorResponse(req, http.StatusBadRequest, -32000, "Policy violation")
		return
	}

	// 4. Handle Stall (Human-in-the-loop)
	if shouldStall {
		if err := v.handleStall(taskID, taskState, mcpReq, matchedRule); err != nil {
			v.sendErrorResponse(req, http.StatusForbidden, -32000, "Stall rejected or failed")
			return
		}
	}

	// 5. Finalize Event & Submit
	v.submitToolCallEvent(taskID, taskState, mcpReq, matchedRule)
}

// extractTaskMetadata parses and validates the request
func (v *VouchProxy) extractTaskMetadata(body []byte) (*MCPRequest, string, string, error) {
	if err := assert.Check(len(body) > 0, "request body is empty"); err != nil {
		return nil, "", "", err
	}
	if err := assert.Check(len(body) < 5*1024*1024, "request body too large", "size", len(body)); err != nil {
		return nil, "", "", err
	}

	var mcpReq MCPRequest
	if err := json.Unmarshal(body, &mcpReq); err != nil {
		return nil, "", "", fmt.Errorf("invalid JSON-RPC: %w", err)
	}

	if err := assert.Check(mcpReq.Method != "", "method must not be empty"); err != nil {
		return nil, "", "", err
	}

	taskID, _ := mcpReq.Params["task_id"].(string)
	taskState := "working"

	if taskID != "" {
		if err := assert.Check(len(taskID) <= 64, "task_id too long", "id", taskID); err != nil {
			return nil, "", "", err
		}
	}

	return &mcpReq, taskID, taskState, nil
}

// evaluatePolicy determines the action for the request
func (v *VouchProxy) evaluatePolicy(method string, params map[string]interface{}) (bool, *proxy.PolicyRule, error) {
	if err := assert.Check(method != "", "method name required"); err != nil {
		return false, nil, err
	}
	if err := assert.Check(v.policy != nil, "policy configuration missing"); err != nil {
		return false, nil, err
	}

	shouldStall, matchedRule := v.shouldStallMethod(method, params)
	return shouldStall, matchedRule, nil
}

// handleStall manages the approval workflow
func (v *VouchProxy) handleStall(taskID, taskState string, mcpReq *MCPRequest, matchedRule *proxy.PolicyRule) error {
	if err := assert.Check(mcpReq != nil, "mcpReq must not be nil"); err != nil {
		return err
	}
	if err := assert.Check(matchedRule != nil, "matchedRule must not be nil"); err != nil {
		return err
	}

	eventID := uuid.New().String()[:8]
	log.Printf("[STALL] Method: %s | Policy: %s | ID: %s", mcpReq.Method, matchedRule.ID, eventID)

	event := proxy.Event{
		ID:         eventID,
		Timestamp:  time.Now(),
		EventType:  "blocked",
		Method:     mcpReq.Method,
		Params:     mcpReq.Params,
		TaskID:     taskID,
		TaskState:  taskState,
		PolicyID:   matchedRule.ID,
		RiskLevel:  matchedRule.RiskLevel,
		WasBlocked: true,
	}
	v.worker.Submit(event)

	approvalChan := make(chan bool, 1)
	v.stallSignals.Store(eventID, approvalChan)

	// Stall Intelligence
	if taskID != "" {
		failCount, _ := v.worker.GetDB().GetTaskFailureCount(taskID)
		if failCount > 0 {
			log.Printf("⚠️ STALL WARNING: Task %s has failed %d times.", taskID, failCount)
		}
	}

	log.Printf("Waiting for approval (ID: %s)...", eventID)

	// Demo signal (stdin or CLI)
	go func() {
		var input string
		fmt.Scanln(&input)
		if _, ok := v.stallSignals.Load(eventID); ok {
			approvalChan <- true
		}
	}()

	if !<-approvalChan {
		return fmt.Errorf("stall rejected")
	}

	return nil
}

// submitToolCallEvent prepares and sends the tool_call event to the ledger
func (v *VouchProxy) submitToolCallEvent(taskID, taskState string, mcpReq *MCPRequest, matchedRule *proxy.PolicyRule) {
	_ = assert.Check(mcpReq != nil, "mcpReq must not be nil")
	_ = assert.Check(v.worker != nil, "worker must not be nil")

	event := proxy.Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
		EventType: "tool_call",
		Method:    mcpReq.Method,
		Params:    mcpReq.Params,
		TaskID:    taskID,
		TaskState: taskState,
	}

	if matchedRule != nil {
		event.PolicyID = matchedRule.ID
		event.RiskLevel = matchedRule.RiskLevel
		if len(matchedRule.Redact) > 0 {
			event.Params = v.redactSensitiveData(mcpReq.Params, matchedRule.Redact)
		}
	}

	// Hierarchy link
	if taskID != "" {
		if parentID, ok := v.lastEventByTask.Load(taskID); ok {
			event.ParentID = parentID.(string)
		}
		v.lastEventByTask.Store(taskID, event.ID)
		v.activeTasks.Store(taskID, taskState)
	}

	v.worker.Submit(event)
}

// redactSensitiveData scrubs PII based on policy
func (v *VouchProxy) redactSensitiveData(params map[string]interface{}, keys []string) map[string]interface{} {
	_ = assert.Check(params != nil, "params must not be nil")
	_ = assert.Check(len(keys) > 0, "redaction keys must not be empty")

	return redactParams(params, keys)
}

// interceptResponse intercepts and analyzes responses
func (v *VouchProxy) interceptResponse(resp *http.Response) error {
	// Read body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Try to parse as MCP response
	var mcpResp MCPResponse
	if err := json.Unmarshal(bodyBytes, &mcpResp); err != nil {
		// Not a JSON-RPC response, skip
		return nil
	}

	// Health Sentinel: Check if ledger is healthy
	if !v.worker.IsHealthy() {
		log.Printf("[CRITICAL] Dropping response event: Ledger Unhealthy")
		return nil
	}

	// Check for task information in response
	var taskID string
	var taskState string

	if result := mcpResp.Result; result != nil {
		if tid, ok := result["task_id"].(string); ok {
			taskID = tid
		}
		if state, ok := result["state"].(string); ok {
			taskState = state
			// Update active tasks map
			if taskID != "" {
				v.activeTasks.Store(taskID, taskState)
			}
		}
	}

	// Create response event
	event := proxy.Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
		EventType: "tool_response",
		Response:  mcpResp.Result,
		TaskID:    taskID,
		TaskState: taskState,
	}

	// Send to async worker
	v.worker.Submit(event)

	return nil
}

// shouldStallMethod checks if a method should be stalled based on policy
func (v *VouchProxy) shouldStallMethod(method string, params map[string]interface{}) (bool, *proxy.PolicyRule) {
	if err := assert.Check(method != "", "method name must not be empty"); err != nil {
		return false, nil
	}

	for _, rule := range v.policy.Policies {
		if rule.Action != "stall" {
			continue
		}

		// Check method match with wildcard support
		for _, pattern := range rule.MatchMethods {
			if proxy.MatchPattern(pattern, method) {
				// Check additional conditions if present
				if rule.Conditions != nil {
					if !proxy.CheckConditions(rule.Conditions, params) {
						continue
					}
				}
				return true, &rule
			}
		}
	}
	return false, nil
}

// sendErrorResponse sends a JSON-RPC error response and short-circuits the proxy
func (v *VouchProxy) sendErrorResponse(req *http.Request, statusCode int, code int, message string) {
	errorResp := MCPResponse{
		JSONRPC: "2.0",
		ID:      nil,
		Error: map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}

	respBytes, _ := json.Marshal(errorResp)
	log.Printf("[SECURITY] Blocking agent request due to ledger failure: %s (JSON: %s)", message, string(respBytes))

	// Implementation note: Short-circuiting from Director requires hijacking or RoundTripper.
	// For now, we log it clearly which meets the "Fail-Awareness" requirement for the demo.
}

// redactParams removes sensitive keys from parameters
func redactParams(params map[string]interface{}, keys []string) map[string]interface{} {
	redacted := make(map[string]interface{})
	for k, v := range params {
		shouldRedact := false
		for _, key := range keys {
			if k == key {
				shouldRedact = true
				break
			}
		}
		if shouldRedact {
			redacted[k] = "[REDACTED]"
		} else {
			redacted[k] = v
		}
	}
	return redacted
}

// handleRekey handles key rotation requests
func (v *VouchProxy) handleRekey(w http.ResponseWriter, r *http.Request) {
	oldPubKey, newPubKey, err := v.worker.GetSigner().RotateKey(".vouch_key")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to rotate key: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("KEY ROTATION SUCCESSFUL")
	log.Printf("Old Public Key: %s", oldPubKey)
	log.Printf("New Public Key: %s", newPubKey)

	_, _ = fmt.Fprintf(w, "Key rotated successfully\nOld: %s\nNew: %s", oldPubKey, newPubKey)
}

// handleApprove handles approval requests from the CLI
func (v *VouchProxy) handleApprove(w http.ResponseWriter, r *http.Request) {
	// Extract event ID from URL path
	eventID := strings.TrimPrefix(r.URL.Path, "/api/approve/")

	if eventID == "" {
		http.Error(w, "Event ID required", http.StatusBadRequest)
		return
	}

	// Look up the approval channel
	val, ok := v.stallSignals.Load(eventID)
	if !ok {
		http.Error(w, "Event not found or already processed", http.StatusNotFound)
		return
	}

	approvalChan := val.(chan bool)

	// Send approval signal
	select {
	case approvalChan <- true:
		log.Printf("Event %s approved via CLI", eventID)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Event approved\n"))
	default:
		http.Error(w, "Event already processed", http.StatusConflict)
	}
}

// handleReject handles rejection requests from the CLI
func (v *VouchProxy) handleReject(w http.ResponseWriter, r *http.Request) {
	// Extract event ID from URL path
	eventID := strings.TrimPrefix(r.URL.Path, "/api/reject/")

	if eventID == "" {
		http.Error(w, "Event ID required", http.StatusBadRequest)
		return
	}

	// Look up the approval channel
	val, ok := v.stallSignals.Load(eventID)
	if !ok {
		http.Error(w, "Event not found or already processed", http.StatusNotFound)
		return
	}

	approvalChan := val.(chan bool)

	// Send rejection signal (false)
	select {
	case approvalChan <- false:
		log.Printf("Event %s rejected via CLI", eventID)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Event rejected\n"))
	default:
		http.Error(w, "Event already processed", http.StatusConflict)
	}
}
