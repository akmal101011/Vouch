package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/slyt3/Vouch/internal/ledger"
	"github.com/slyt3/Vouch/internal/proxy"
)

func TraceCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: vouch trace <task-id>")
		os.Exit(1)
	}
	taskID := os.Args[2]

	db, err := ledger.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	events, err := db.GetEventsByTaskID(taskID)
	if err != nil {
		log.Fatalf("Failed to get events: %v", err)
	}

	if len(events) == 0 {
		fmt.Printf("No events found for task %s\n", taskID)
		return
	}

	fmt.Printf("Forensic Timeline Trace: %s\n", taskID)
	fmt.Printf("Run ID: %s\n", events[0].RunID[:8])
	fmt.Printf("Start:  %s\n", events[0].Timestamp.Format(time.RFC3339))
	fmt.Println(strings.Repeat("=", 60))

	// Reconstruct Hierarchy
	roots, childrenMap := buildTree(events)

	// Visualize
	for _, root := range roots {
		printTraceNode(root, childrenMap, "", true)
	}
}

func buildTree(events []proxy.Event) ([]proxy.Event, map[string][]proxy.Event) {
	childrenMap := make(map[string][]proxy.Event)
	var roots []proxy.Event

	// Index by ID for quick lookup if needed, but here we just need parent links
	// Assuming events are ordered by sequence (time)

	for _, e := range events {
		if e.ParentID == "" {
			roots = append(roots, e)
		} else {
			childrenMap[e.ParentID] = append(childrenMap[e.ParentID], e)
		}
	}
	return roots, childrenMap
}

func printTraceNode(e proxy.Event, childrenMap map[string][]proxy.Event, prefix string, isLast bool) {
	// Marker symbols
	marker := "├── "
	if isLast {
		marker = "└── "
	}

	// Status icon
	statusSym := "○" // Default: Call
	if e.EventType == "tool_response" {
		statusSym = "●" // Response
	}
	if e.WasBlocked {
		statusSym = "×" // Blocked
	}
	if e.RiskLevel == "critical" {
		statusSym = "‼" // Critical
	}

	// Format Timestamp delta (not implemented here for brevity, simple print)

	fmt.Printf("%s%s%s %s [%s]\n", prefix, marker, statusSym, e.Method, e.ID[:6])

	// New Prefix for children
	newPrefix := prefix
	if isLast {
		newPrefix += "    "
	} else {
		newPrefix += "│   "
	}

	children := childrenMap[e.ID]
	for i, child := range children {
		printTraceNode(child, childrenMap, newPrefix, i == len(children)-1)
	}

}
