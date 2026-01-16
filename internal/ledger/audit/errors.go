package audit

import "errors"

var (
	ErrChainTampered    = errors.New("forensic integrity error: hash chain link broken")
	ErrInvalidSignature = errors.New("forensic integrity error: invalid ed25519 signature")
	ErrHashMismatch     = errors.New("forensic integrity error: hash mismatch (data tampered)")
	ErrNoEvents         = errors.New("forensic integrity error: no events found in ledger")
)
