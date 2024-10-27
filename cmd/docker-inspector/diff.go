// Package diff provides functionality for comparing Docker image contents
package main

import (
	"fmt"
	"strings"
	"time"
)

// Mode specifies what attributes to compare
type Mode int

const (
	// CompareAll includes all attributes including modification times
	CompareAll Mode = iota
	// CompareNoTimes excludes modification time comparisons
	CompareNoTimes
)

// Change represents the type of difference found
type Change string

const (
	Added    Change = "added"
	Removed  Change = "removed"
	Modified Change = "modified"
)

// FileDiff represents a difference between two versions of a file
type FileDiff struct {
	Path    string   `json:"path"`
	Type    Change   `json:"type"`
	OldFile FileInfo `json:"oldFile,omitempty"`
	NewFile FileInfo `json:"newFile,omitempty"`
	// Details contains human-readable descriptions of the changes
	Details []string `json:"details,omitempty"`
}

// Summary contains statistical information about the differences
type Summary struct {
	TotalDifferences int `json:"totalDifferences"`
	AddedFiles       int `json:"addedFiles"`
	RemovedFiles     int `json:"removedFiles"`
	ModifiedFiles    int `json:"modifiedFiles"`
}

// Result contains the complete diff information
type Result struct {
	Differences []FileDiff `json:"differences"`
	Summary     Summary    `json:"summary"`
}

// FileInfo mirrors the internal inspector's FileInfo structure
type FileInfo struct {
	Path      string     `json:"path"`
	Size      int64      `json:"size"`
	Mode      string     `json:"mode"`
	ModTime   *time.Time `json:"modTime,omitempty"`
	IsDir     bool       `json:"isDir"`
	SymlinkTo string     `json:"symlinkTo,omitempty"`
	User      string     `json:"user"`
	Group     string     `json:"group"`
	MD5       string     `json:"md5,omitempty"`
}

// Compare performs a comparison of two sets of FileInfo records
func Compare(old, new []FileInfo, mode Mode) (*Result, error) {
	result := &Result{}

	// Create maps for faster lookups
	oldFiles := make(map[string]FileInfo)
	newFiles := make(map[string]FileInfo)

	// Skip special files and populate maps
	for _, f := range old {
		if !isSpecialFile(f.Path) {
			oldFiles[f.Path] = f
		}
	}
	for _, f := range new {
		if !isSpecialFile(f.Path) {
			newFiles[f.Path] = f
		}
	}

	// Find removed files
	for path, oldFile := range oldFiles {
		if _, exists := newFiles[path]; !exists {
			diff := FileDiff{
				Path:    path,
				Type:    Removed,
				OldFile: oldFile,
			}
			result.Differences = append(result.Differences, diff)
			result.Summary.RemovedFiles++
		}
	}

	// Find added and modified files
	for path, newFile := range newFiles {
		oldFile, exists := oldFiles[path]
		if !exists {
			diff := FileDiff{
				Path:    path,
				Type:    Added,
				NewFile: newFile,
			}
			result.Differences = append(result.Differences, diff)
			result.Summary.AddedFiles++
			continue
		}

		// Check for modifications
		if differences := compareFiles(oldFile, newFile, mode); len(differences) > 0 {
			diff := FileDiff{
				Path:    path,
				Type:    Modified,
				OldFile: oldFile,
				NewFile: newFile,
				Details: differences,
			}
			result.Differences = append(result.Differences, diff)
			result.Summary.ModifiedFiles++
		}
	}

	result.Summary.TotalDifferences = result.Summary.AddedFiles +
		result.Summary.RemovedFiles +
		result.Summary.ModifiedFiles

	return result, nil
}

// compareFiles returns a list of differences between two files
func compareFiles(old, new FileInfo, mode Mode) []string {
	var differences []string

	// Compare basic attributes
	if old.Size != new.Size {
		differences = append(differences,
			fmt.Sprintf("size changed: %d -> %d", old.Size, new.Size))
	}
	if old.Mode != new.Mode {
		differences = append(differences,
			fmt.Sprintf("permissions changed: %s -> %s", old.Mode, new.Mode))
	}
	if old.User != new.User || old.Group != new.Group {
		differences = append(differences,
			fmt.Sprintf("ownership changed: %s:%s -> %s:%s",
				old.User, old.Group, new.User, new.Group))
	}

	// Compare modification times if requested
	if mode == CompareAll && old.ModTime != nil && new.ModTime != nil {
		if !old.ModTime.Equal(*new.ModTime) {
			differences = append(differences,
				fmt.Sprintf("modification time changed: %s -> %s",
					old.ModTime.Format(time.RFC3339),
					new.ModTime.Format(time.RFC3339)))
		}
	}

	// Compare MD5 if available
	if old.MD5 != "" && new.MD5 != "" && old.MD5 != new.MD5 {
		differences = append(differences, "content changed (different MD5)")
	}

	return differences
}

// isSpecialFile returns true for files we want to ignore
func isSpecialFile(path string) bool {
	return strings.HasPrefix(path, "/proc/") ||
		strings.HasPrefix(path, "/sys/") ||
		strings.HasPrefix(path, "/dev/") ||
		path == "/etc/resolv.conf" ||
		path == "/etc/hostname" ||
		path == "/etc/hosts"
}
