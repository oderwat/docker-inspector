//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func isOwnershipSupported(dir string) bool {
	// Convert to absolute path
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return false
	}

	created := false
	stat, err := os.Stat(absPath)
	if err != nil {
		// Create the output directory if it doesn't exist
		if err := os.Mkdir(absPath, 0755); err != nil {
			return false
		}
		created = true
	}
	defer func() {
		if created {
			if err := os.Remove(absPath); err != nil {
				fmt.Fprintf(os.Stderr, "failed to remove %q: %v", absPath, err)
			}
		}
	}()

	testFile, err := os.CreateTemp(dir, ".ownership-test-*")
	if err != nil {
		return false
	}
	testPath := testFile.Name()
	testFile.Close()
	defer os.Remove(testPath)

	fmt.Fprintf(os.Stderr, "Checking filesystem of %q for ownership support (requires sudo)...\n", dir)
	// Try to change ownership to root:root
	if err := exec.Command("sudo", "chown", "999:999", testPath).Run(); err != nil {
		return false
	}

	// Read back the ownership
	stat, err = os.Stat(testPath)
	if err != nil {
		return false
	}

	if sys, ok := stat.Sys().(*syscall.Stat_t); ok {
		return sys.Uid == 999 && sys.Gid == 999
	}
	return false
}
