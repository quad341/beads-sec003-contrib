package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateRunID mints a REPLAY_RUN id. The "replay-run-" prefix is
// structurally outside the dolt-commit-hash alphabet ([0-9a-v]), so the
// result can never collide with, or be mistaken for, a bare Dolt commit hash
// (NFR5/R2) regardless of what the random suffix happens to contain.
func GenerateRunID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("replay-run-%x", time.Now().UnixNano())
	}
	return "replay-run-" + hex.EncodeToString(buf[:])
}
