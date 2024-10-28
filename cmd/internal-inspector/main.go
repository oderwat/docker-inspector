package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/alexflint/go-arg"
	"github.com/bmatcuk/doublestar/v4"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
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
	Path                string `arg:"--path" default:"/" help:"path to inspect"`
	Pattern             string `arg:"--glob" help:"glob pattern for matching files (supports **/)"`
	MD5                 bool   `arg:"--md5" help:"calculate MD5 checksums for files"`
	NoTimes             bool   `arg:"--no-times" help:"exclude modification times from output"`
	OutputDir           string `arg:"--output-dir" help:"extract matching files to this directory"`
	StripComponents     int    `arg:"--strip-components" help:"strip NUMBER leading components from file names"`
	PreserveOwner       bool   `arg:"--preserve-owner" help:"preserve user/group information when extracting"`
	PreservePermissions bool   `arg:"--preserve-perms" help:"preserve file perms when extracting"`
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
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Cannot access %s: %v\n", path, err)
			skippedCount++
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// We always need to skip some directories
		if path == "/inspect-target" ||
			path == "/proc" ||
			path == "/sys" ||
			path == "/dev" {
			skippedCount++
			//fmt.Fprintf(os.Stderr, "Skipping %s:\n", path)
			return filepath.SkipDir
		}
		// We always need to skip our inspector
		if path == "/inspect" {
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

		// Get user and group information
		userName, groupName, err := getUserGroupNames(info)
		if err != nil {
			userName = "unknown"
			groupName = "unknown"
		}

		fileInfo := FileInfo{
			Path:      path,
			Size:      info.Size(),
			Mode:      info.Mode().String(),
			IsDir:     info.IsDir(),
			SymlinkTo: symlinkTo,
			User:      userName,
			Group:     groupName,
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

	// If output directory is specified, copy matching files
	if args.OutputDir != "" {
		for _, file := range files {
			if file.IsDir {
				continue // Skip directories, they'll be created as needed
			}

			destPath := getDestPath(file.Path, args.StripComponents)
			if destPath == "" {
				continue // Skip if all components were stripped
			}

			fullDestPath := filepath.Join(args.OutputDir, destPath)

			info, err := os.Lstat(file.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Cannot stat %s: %v\n", file.Path, err)
				continue
			}

			if err := copyFile(file.Path, fullDestPath, info,
				args.PreservePermissions,
				args.PreserveOwner); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to copy %s: %v\n", file.Path, err)
				continue
			}
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(files)
}

func copyFile(src string, dest string, info fs.FileInfo, preservePerms, preserveUser bool) error {
	// Create destination directory if it doesn't exist
	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %v", err)
	}

	// Handle symlinks
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %v", err)
		}
		return os.Symlink(target, dest)
	}

	// Copy regular file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer srcFile.Close()
	// Create destination file
	destFile, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %v", err)
	}

	// Get original file's stats
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to get stat info")
	}

	if preservePerms {
		//fmt.Fprintf(os.Stderr, "Debug: Setting mode on %s to %s\n", dest, info.Mode())
		if err := os.Chmod(dest, info.Mode()); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not preserve mode of %s: %v\n", dest, err)
		}
	}

	if preserveUser {
		uid := int(stat.Uid)
		gid := int(stat.Gid)
		//fmt.Fprintf(os.Stderr, "Debug: Attempting to set ownership on %s to %d:%d\n", dest, uid, gid)
		if err := os.Chown(dest, uid, gid); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not preserve ownership of %s: %v\n", dest, err)
		}
	}

	// Verify final state if debugging
	if destInfo, err := os.Lstat(dest); err == nil {
		//fmt.Fprintf(os.Stderr, "Debug: Final mode: %s\n", destInfo.Mode())
		if destStat, ok := destInfo.Sys().(*syscall.Stat_t); ok {
			//fmt.Fprintf(os.Stderr, "Debug: Final uid:gid = %d:%d\n", destStat.Uid, destStat.Gid)
			if destStat.Uid != uint32(stat.Uid) || destStat.Gid != uint32(stat.Gid) {
				fmt.Fprintf(os.Stderr, "Warning: Final ownership is %d:%d but %d:%d was expected\n",
					destStat.Uid, destStat.Gid, stat.Uid, stat.Gid)
			}
		}
	}

	return nil
}

func getDestPath(sourcePath string, stripComponents int) string {
	// Split path into components
	parts := strings.Split(strings.TrimPrefix(sourcePath, "/"), "/")

	// Strip leading components
	if stripComponents >= len(parts) {
		return ""
	}

	return "/" + filepath.Join(parts[stripComponents:]...)
}

// Add a helper function to get user and group names with IDs
func getUserGroupNames(info fs.FileInfo) (string, string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", fmt.Errorf("failed to get stat info")
	}

	uid := stat.Uid
	gid := stat.Gid

	// Try to lookup user
	userName := strconv.FormatUint(uint64(uid), 10) // Default to just the ID
	if u, err := user.LookupId(userName); err == nil {
		userName = fmt.Sprintf("%s(%d)", u.Username, uid)
	} else {
		userName = fmt.Sprintf("(%d)", uid) // Just ID in parentheses if no name found
	}

	// Try to lookup group
	groupName := strconv.FormatUint(uint64(gid), 10) // Default to just the ID
	if g, err := user.LookupGroupId(groupName); err == nil {
		groupName = fmt.Sprintf("%s(%d)", g.Name, gid)
	} else {
		groupName = fmt.Sprintf("(%d)", gid) // Just ID in parentheses if no name found
	}

	return userName, groupName, nil
}
