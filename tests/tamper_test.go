package tests

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/slyt3/Vouch/internal/ledger"
	"github.com/slyt3/Vouch/internal/ledger/audit"
	"github.com/slyt3/Vouch/internal/ledger/store"
	"github.com/slyt3/Vouch/internal/models"
)

func TestTamperDetection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vouch-tamper-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "vouch_tamper.db")
	keyPath := filepath.Join(tempDir, "test.key")

	// Copy schema.sql to temp dir for NewDB to find it
	schemaContent, err := os.ReadFile("../schema.sql")
	if err != nil {
		t.Fatalf("failed to read schema: %v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "schema.sql"), schemaContent, 0644)
	if err != nil {
		t.Fatalf("failed to write schema: %v", err)
	}

	// 1. Setup Ledger and Signer
	// Need to set Cwd to tempDir so worker finds schema.sql
	origWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(origWd)

	dbStore, err := store.NewDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	l, err := ledger.NewWorker(10, dbStore, keyPath)
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}
	if err := l.Start(); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}
	db := l.GetDB()
	runID, err := db.GetRunID()
	if err != nil {
		t.Fatalf("failed to get run ID: %v", err)
	}

	// 2. Insert a few valid events
	processor := ledger.NewEventProcessor(db, l.GetSigner(), runID)
	for i := 0; i < 3; i++ {
		e := &models.Event{
			ID:        uuid.New().String()[:8],
			Timestamp: time.Now(),
			Actor:     "agent",
			EventType: "tool_call",
			Method:    "os.read",
		}
		if err := processor.ProcessEvent(e); err != nil {
			t.Fatalf("failed to process event %d: %v", i, err)
		}
	}

	// Verify initially valid
	res, err := audit.VerifyChain(db, runID, l.GetSigner())
	if err != nil || !res.Valid {
		t.Fatalf("initial chain invalid: %v (msg: %s)", err, res.ErrorMessage)
	}

	// Helper to get raw SQL connection for tampering
	rawDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open raw db: %v", err)
	}
	defer rawDB.Close()

	t.Run("DetectHashMismatch", func(t *testing.T) {
		// Tamper with the params of seq_index 1
		_, err := rawDB.Exec("UPDATE events SET method = 'TAMPERED' WHERE seq_index = 1 AND run_id = ?", runID)
		if err != nil {
			t.Fatalf("failed to tamper: %v", err)
		}

		res, err := audit.VerifyChain(db, runID, l.GetSigner())
		if err != nil {
			t.Fatalf("VerifyChain failed: %v", err)
		}
		if res.Valid {
			t.Error("expected chain to be invalid after hash tampering")
		}
		if !strings.Contains(res.ErrorMessage, audit.ErrHashMismatch.Error()) {
			t.Errorf("expected error %v in %s", audit.ErrHashMismatch, res.ErrorMessage)
		}

		// Restore
		_, _ = rawDB.Exec("UPDATE events SET method = 'os.read' WHERE seq_index = 1 AND run_id = ?", runID)
	})

	t.Run("DetectChainLinkageBreak", func(t *testing.T) {
		// Tamper with the prev_hash of seq_index 2
		_, err := rawDB.Exec("UPDATE events SET prev_hash = 'WRONG_HASH' WHERE seq_index = 2 AND run_id = ?", runID)
		if err != nil {
			t.Fatalf("failed to tamper: %v", err)
		}

		res, err := audit.VerifyChain(db, runID, l.GetSigner())
		if err != nil {
			t.Fatalf("VerifyChain failed: %v", err)
		}
		if res.Valid {
			t.Error("expected chain to be invalid after prev_hash tampering")
		}
		if res.ErrorMessage != audit.ErrChainTampered.Error() {
			t.Errorf("expected error %v, got %s", audit.ErrChainTampered, res.ErrorMessage)
		}

		// Restore (approximate, since we don't store previous hash separately, we'd need to query seq 1)
		var seq1Hash string
		_ = rawDB.QueryRow("SELECT current_hash FROM events WHERE seq_index = 1 AND run_id = ?", runID).Scan(&seq1Hash)
		_, _ = rawDB.Exec("UPDATE events SET prev_hash = ? WHERE seq_index = 2 AND run_id = ?", seq1Hash, runID)
	})

	t.Run("DetectInvalidSignature", func(t *testing.T) {
		// Tamper with the signature of seq_index 1
		_, err := rawDB.Exec("UPDATE events SET signature = 'INVALID_SIG' WHERE seq_index = 1 AND run_id = ?", runID)
		if err != nil {
			t.Fatalf("failed to tamper: %v", err)
		}

		res, err := audit.VerifyChain(db, runID, l.GetSigner())
		if err != nil {
			t.Fatalf("VerifyChain failed: %v", err)
		}
		if res.Valid {
			t.Error("expected chain to be invalid after signature tampering")
		}
		if !strings.Contains(res.ErrorMessage, audit.ErrInvalidSignature.Error()) {
			t.Errorf("expected error %v in %s", audit.ErrInvalidSignature, res.ErrorMessage)
		}
	})
}
