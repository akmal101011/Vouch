package assert

import (
	"fmt"
	"log"
	"runtime/debug"
)

// Mode defines the behavior of the assertion library
// In strict mode (default), assertions panic.
var StrictMode = true

// Check verifies a condition is true. If false, it logs the error and
// in strict mode, panics. This enforces the "Fail Fast" rule.
// Use this for preconditions, postconditions, and invariants.
func Check(condition bool, msg string, args ...interface{}) error {
	if condition {
		return nil
	}

	formattedMsg := fmt.Sprintf(msg, args...)
	err := fmt.Errorf("ASSERTION FAILED: %s", formattedMsg)

	log.Printf("[CRITICAL] %v\nStack: %s", err, debug.Stack())

	if StrictMode {
		panic(err)
	}

	return err
}

// NotNil checks that a pointer or interface is not nil.
func NotNil(obj interface{}, name string) {
	Check(obj != nil, "%s must not be nil", name)
}

// InRange checks that value is within [min, max] inclusive.
func InRange(val, min, max int, name string) {
	Check(val >= min && val <= max, "%s (%d) out of range [%d, %d]", name, val, min, max)
}

// True is an alias for Check, for readability.
func True(condition bool, msg string, args ...interface{}) {
	_ = Check(condition, msg, args...)
}
