package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/bmatcuk/doublestar/v4"
)

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

type Args struct {
	Path    string `arg:"--path" default:"/" help:"path to inspect"`
	Pattern string `arg:"--glob" help:"glob pattern for matching files (supports **/)"`
	MD5     bool   `arg:"--md5" help:"calculate MD5 checksums for files"`
	NoTimes bool   `arg:"--no-times" help:"exclude modification times from output"`
}

func calculateMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func main() {
	var args Args
	// Set defaults
	args.Path = "/"

	arg.MustParse(&args)

	var files []FileInfo
	var totalSize int64
	var dirCount, fileCount, md5Count, md5ErrorCount, skippedCount int

	err := filepath.Walk(args.Path, func(path string, info fs.FileInfo, err error) error {
		// Handle path errors gracefully
		if strings.HasPrefix(path, "/proc") ||
			strings.HasPrefix(path, "/sys") ||
			strings.HasPrefix(path, "/dev") {
			skippedCount++
			//fmt.Fprintf(os.Stderr, "Skipping %s:\n", path)
			return filepath.SkipDir
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Cannot access %s: %v\n", path, err)
			skippedCount++
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Pattern matching if specified
		if args.Pattern != "" {
			match, err := doublestar.Match(args.Pattern, path)
			if err != nil {
				return fmt.Errorf("invalid pattern: %v", err)
			}
			if !match {
				return nil
			}
		}

		// Get symlink target if it's a symlink
		symlinkTo := ""
		if info.Mode()&os.ModeSymlink != 0 {
			symlinkTo, _ = os.Readlink(path)
		}

		// Count files and directories
		if info.IsDir() {
			dirCount++
		} else {
			fileCount++
		}

		totalSize += info.Size()

		fileInfo := FileInfo{
			Path:      path,
			Size:      info.Size(),
			Mode:      info.Mode().String(),
			IsDir:     info.IsDir(),
			SymlinkTo: symlinkTo,
			User:      "root",
			Group:     "root",
		}

		if !args.NoTimes {
			modTime := info.ModTime()
			fileInfo.ModTime = &modTime
		}

		// Calculate MD5 if requested and file is not a directory
		if args.MD5 && !info.IsDir() && info.Size() > 0 && symlinkTo == "" {
			if md5sum, err := calculateMD5(path); err == nil {
				fileInfo.MD5 = md5sum
				md5Count++
			} else {
				md5ErrorCount++
				fileInfo.MD5 = fmt.Sprintf("error: %v", err)
			}
		}

		files = append(files, fileInfo)
		return nil
	})

	// Change the error handling at the Walk level
	if err != nil && !os.IsPermission(err) && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Sort by path for consistent output
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(files)
}
