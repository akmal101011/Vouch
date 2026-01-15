package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/yourname/ael/internal/crypto"
	"github.com/yourname/ael/internal/ledger"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "verify":
		verifyCommand()
	case "status":
		statusCommand()
	case "events":
		eventsCommand()
	case "approve":
		approveCommand()
	case "reject":
		rejectCommand()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("AEL CLI - Agent Execution Ledger Command Line Tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ael verify              Validate the entire hash chain")
	fmt.Println("  ael status              Show current run information")
	fmt.Println("  ael events [--limit N]  List recent events (default: 10)")
	fmt.Println("  ael approve <event-id>  Approve a stalled action")
	fmt.Println("  ael reject <event-id>   Reject a stalled action")
}

func verifyCommand() {
	// Open database
	db, err := ledger.NewDB("ael.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Load signer
	signer, err := crypto.NewSigner(".ael_key")
	if err != nil {
		log.Fatalf("Failed to load signer: %v", err)
	}

	// Get current run ID
	runID, err := db.GetRunID()
	if err != nil {
		log.Fatalf("Failed to get run ID: %v", err)
	}

	if runID == "" {
		fmt.Println("No runs found in database")
		return
	}

	fmt.Printf("Verifying chain for run: %s\n", runID[:8])

	// Verify chain
	result, err := ledger.VerifyChain(db, runID, signer)
	if err != nil {
		log.Fatalf("Verification error: %v", err)
	}

	if result.Valid {
		fmt.Printf("✓ Chain is valid (%d events verified)\n", result.TotalEvents)
	} else {
		fmt.Printf("✗ Chain verification failed\n")
		fmt.Printf("  Error: %s\n", result.ErrorMessage)
		if result.FailedAtSeq > 0 {
			fmt.Printf("  Failed at sequence: %d\n", result.FailedAtSeq)
		}
		os.Exit(1)
	}
}

func statusCommand() {
	// Open database
	db, err := ledger.NewDB("ael.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Get current run ID
	runID, err := db.GetRunID()
	if err != nil {
		log.Fatalf("Failed to get run ID: %v", err)
	}

	if runID == "" {
		fmt.Println("No runs found in database")
		return
	}

	// Get run info
	agentName, genesisHash, pubKey, err := db.GetRunInfo(runID)
	if err != nil {
		log.Fatalf("Failed to get run info: %v", err)
	}

	fmt.Println("Current Run Status")
	fmt.Println("==================")
	fmt.Printf("Run ID:       %s\n", runID[:8])
	fmt.Printf("Agent:        %s\n", agentName)
	fmt.Printf("Genesis Hash: %s\n", genesisHash[:16]+"...")
	fmt.Printf("Public Key:   %s\n", pubKey[:32]+"...")
}

func eventsCommand() {
	// Parse flags
	eventsFlags := flag.NewFlagSet("events", flag.ExitOnError)
	limit := eventsFlags.Int("limit", 10, "Number of events to show")
	eventsFlags.Parse(os.Args[2:])

	// Open database
	db, err := ledger.NewDB("ael.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Get current run ID
	runID, err := db.GetRunID()
	if err != nil {
		log.Fatalf("Failed to get run ID: %v", err)
	}

	if runID == "" {
		fmt.Println("No runs found in database")
		return
	}

	// Get recent events
	events, err := db.GetRecentEvents(runID, *limit)
	if err != nil {
		log.Fatalf("Failed to get events: %v", err)
	}

	fmt.Printf("Recent Events (showing %d)\n", len(events))
	fmt.Println("===========================")
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		fmt.Printf("[%d] %s | %s | %s\n", e.SeqIndex, e.ID[:8], e.EventType, e.Method)
		if e.WasBlocked {
			fmt.Printf("    BLOCKED\n")
		}
	}
}

func approveCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: ael approve <event-id>")
		os.Exit(1)
	}

	eventID := os.Args[2]
	fmt.Printf("Approving event: %s\n", eventID)
	fmt.Println("Note: HTTP approval endpoint not yet implemented")
	fmt.Println("Use the proxy's stdin for now (press Enter)")
}

func rejectCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: ael reject <event-id>")
		os.Exit(1)
	}

	eventID := os.Args[2]
	fmt.Printf("Rejecting event: %s\n", eventID)
	fmt.Println("Note: HTTP rejection endpoint not yet implemented")
}
