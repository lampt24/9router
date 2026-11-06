//go:build !unix

package server

import "sync"

// The server only runs on Linux; this keeps the package building elsewhere
// (single-process locking only).
var mu sync.Mutex

func lockFile(string) (func(), error) {
	mu.Lock()
	return mu.Unlock, nil
}
