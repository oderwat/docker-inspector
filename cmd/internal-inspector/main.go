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
	"text/tabwriter"
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
	JSON    bool   `arg:"--json" help:"output in JSON format"`
	Summary bool   `arg:"--summary" help:"show summary statistics"`
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
	args.Summary = false
	args.Path = "/"

	arg.MustParse(&args)

	var files []FileInfo
	var totalSize int64
	var dirCount, fileCount, md5Count, md5ErrorCount, skippedCount int

	err := filepath.Walk(args.Path, func(path string, info fs.FileInfo, err error) error {
		// Handle path errors gracefully
		if err != nil {
			// Skip special filesystem errors
			if os.IsNotExist(err) ||
				strings.HasPrefix(path, "/proc") ||
				strings.HasPrefix(path, "/sys") ||
				strings.HasPrefix(path, "/dev") {
				skippedCount++
				return filepath.SkipDir
			}
			// For other errors, log and continue
			fmt.Fprintf(os.Stderr, "Warning: Cannot access %s: %v\n", path, err)
			skippedCount++
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
		if args.MD5 && !info.IsDir() {
			if md5sum, err := calculateMD5(path); err == nil {
				fileInfo.MD5 = md5sum
				md5Count++
			} else {
				md5ErrorCount++
				if args.JSON {
					fileInfo.MD5 = fmt.Sprintf("error: %v", err)
				}
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

	// Output results
	if args.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(files)
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
		header := "Mode\tSize\tModified\tUser\tGroup\tPath\tSymlink"
		if args.MD5 {
			header += "\tMD5"
		}
		fmt.Fprintln(w, header)

		for _, file := range files {
			symlink := ""
			if file.SymlinkTo != "" {
				symlink = "-> " + file.SymlinkTo
			}
			// Build the line string, conditionally including the time field.
			// When NoTimes is true, timeStr will be empty and won't add a tab,
			// otherwise it adds both the formatted time and a tab.
			timeStr := ""
			if !args.NoTimes {
				timeStr = file.ModTime.Format("2006-01-02 15:04:05") + "\t"
			}
			line := fmt.Sprintf("%s\t%d\t%s%s\t%s\t%s\t%s",
				file.Mode,
				file.Size,
				timeStr,
				file.User,
				file.Group,
				file.Path,
				symlink,
			)
			if args.MD5 {
				line += fmt.Sprintf("\t%s", file.MD5)
			}
			fmt.Fprintln(w, line)
		}
		w.Flush()
	}

	// Print summary if requested
	if args.Summary {
		out := os.Stdout
		if args.JSON {
			out = os.Stderr
		}
		fmt.Fprintf(out, "\nSummary:\n")
		fmt.Fprintf(out, "Total size: %d bytes\n", totalSize)
		fmt.Fprintf(out, "Directories: %d\n", dirCount)
		fmt.Fprintf(out, "Files: %d\n", fileCount)
		if skippedCount > 0 {
			fmt.Fprintf(out, "Skipped items: %d\n", skippedCount)
		}
		if args.MD5 {
			fmt.Fprintf(out, "MD5 checksums calculated: %d\n", md5Count)
			if md5ErrorCount > 0 {
				fmt.Fprintf(out, "MD5 calculation errors: %d\n", md5ErrorCount)
			}
		}
	}
}
