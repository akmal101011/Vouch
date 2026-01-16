package store

import (
	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/ledger"
)

// GetRunStats returns statistics for a specific run
func (db *DB) GetRunStats(runID string) (*ledger.RunStats, error) {
	if err := assert.Check(runID != "", "runID must not be empty"); err != nil {
		return nil, err
	}
	stats := &ledger.RunStats{
		RunID:         runID,
		RiskBreakdown: make(map[string]int),
	}

	// Total and Blocked counts
	err := db.conn.QueryRow(`
		SELECT COUNT(*), 
		       COALESCE(SUM(CASE WHEN event_type = 'blocked' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN event_type = 'tool_call' THEN 1 ELSE 0 END), 0)
		FROM events WHERE run_id = ?`, runID).Scan(&stats.TotalEvents, &stats.BlockedCount, &stats.CallCount)
	if err != nil {
		return nil, err
	}

	// Risk breakdown
	rows, err := db.conn.Query(`
		SELECT risk_level, COUNT(*) FROM events 
		WHERE run_id = ? AND risk_level != '' 
		GROUP BY risk_level`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var risk string
		var count int
		if err := rows.Scan(&risk, &count); err == nil {
			stats.RiskBreakdown[risk] = count
		}
	}

	return stats, nil
}

// GetGlobalStats returns overall statistics
func (db *DB) GetGlobalStats() (*ledger.GlobalStats, error) {
	stats := &ledger.GlobalStats{}

	err := db.conn.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&stats.TotalRuns)
	if err != nil {
		return nil, err
	}

	err = db.conn.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&stats.TotalEvents)
	if err != nil {
		return nil, err
	}

	err = db.conn.QueryRow(`SELECT COUNT(*) FROM events WHERE risk_level = 'critical'`).Scan(&stats.CriticalCount)
	if err != nil {
		return nil, err
	}

	return stats, nil
}
