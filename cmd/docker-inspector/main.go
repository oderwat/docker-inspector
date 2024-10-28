package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/alexflint/go-arg"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
)

//go:embed internal-inspector
var internalInspector []byte

type Args struct {
	Image1  string `arg:"positional,required" help:"docker image to inspect (or first image when comparing)"`
	Image2  string `arg:"positional" help:"second docker image (for comparison mode)"`
	Path    string `arg:"--path" default:"/" help:"path inside the container to inspect"`
	JSON    bool   `arg:"--json" help:"output in JSON format"`
	Summary bool   `arg:"--summary" help:"show summary statistics"`
	Pattern string `arg:"--glob" help:"glob pattern for matching files (supports **/)"`
	MD5     bool   `arg:"--md5" help:"calculate MD5 checksums for files"`
	Keep    bool   `arg:"--keep" help:"keep the temporary container after inspection"`
	NoTimes bool   `arg:"--no-times" help:"exclude modification times from output"`
	// for extraction
	OutputDir           string `arg:"--output-dir" help:"extract matching files to this directory"`
	StripComponents     int    `arg:"--strip-components" help:"strip NUMBER leading components from file names"`
	PreserveOwner       bool   `arg:"--preserve-owner" help:"preserve user/group information when extracting"`
	PreservePermissions bool   `arg:"--preserve-perms" help:"preserve file permissions when extracting"`
	PreserveAll         bool   `arg:"--preserve-all" help:"preserve all file attributes"`
}

func (Args) Version() string {
	return "docker-inspector 1.1.0"
}

func (Args) Description() string {
	return "Docker image content inspector - examines, extracts and compares files inside container images"
}

func printDiffText(result *Result) {
	// Print summary
	fmt.Printf("\nComparison Summary:\n")
	fmt.Printf("Total differences: %d\n", result.Summary.TotalDifferences)
	fmt.Printf("Added files: %d\n", result.Summary.AddedFiles)
	fmt.Printf("Removed files: %d\n", result.Summary.RemovedFiles)
	fmt.Printf("Modified files: %d\n\n", result.Summary.ModifiedFiles)

	// Print detailed differences
	if len(result.Differences) > 0 {
		fmt.Println("Details:")
		for _, diff := range result.Differences {
			switch diff.Type {
			case Added:
				fmt.Printf("+ %s\n", diff.Path)
				fmt.Printf("  (%d bytes, %s:%s, mode %s)\n",
					diff.NewFile.Size, diff.NewFile.User, diff.NewFile.Group, diff.NewFile.Mode)
			case Removed:
				fmt.Printf("- %s\n", diff.Path)
				fmt.Printf("  (%d bytes, %s:%s, mode %s)\n",
					diff.OldFile.Size, diff.OldFile.User, diff.OldFile.Group, diff.OldFile.Mode)
			case Modified:
				fmt.Printf("M %s\n", diff.Path)
				for _, detail := range diff.Details {
					fmt.Printf("  %s\n", detail)
				}
			}
		}
	}
}

func runInspector(image string, args Args) ([]byte, error) {
	// Create a temporary directory for the inspector
	tempDir, err := os.MkdirTemp("", "docker-inspector-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Write the embedded Linux inspector to the temp directory with executable permissions
	inspectorPath := filepath.Join(tempDir, "internal-inspector")
	if err := os.WriteFile(inspectorPath, internalInspector, 0755); err != nil {
		return nil, fmt.Errorf("failed to write inspector: %v", err)
	}

	// Ensure the inspector is executable on the host
	if err := os.Chmod(inspectorPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to make inspector executable: %v", err)
	}

	// Start building the docker run command
	dockerArgs := []string{"run"}
	if !args.Keep {
		dockerArgs = append(dockerArgs, "--rm")
	}

	// If output directory is specified, mount it
	if args.OutputDir != "" {
		// Convert to absolute path
		absPath, err := filepath.Abs(args.OutputDir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for output dir: %v", err)
		}

		// Create the output directory if it doesn't exist
		if err := os.Mkdir(absPath, 0755); err != nil && !os.IsExist(err) {
			return nil, fmt.Errorf("failed to create output directory: %v", err)
		}

		dockerArgs = append(dockerArgs,
			"-v", fmt.Sprintf("%s:/inspect-target", absPath))
	}

	/*
		// Add capabilities if we need to preserve ownership
		if args.OutputDir != "" && args.PreserveOwner {
			// Option 1: Full privileged mode (more than we need, but guaranteed to work)
			//dockerArgs = append(dockerArgs, "--privileged")
				// Option 2: Just the capabilities we need (more secure)
				dockerArgs = append(dockerArgs,
					"--cap-add=CHOWN",
					"--cap-add=DAC_OVERRIDE",
					"--cap-add=DAC_READ_SEARCH")
		}
	*/

	// Mount the inspector and set it as entrypoint
	dockerArgs = append(dockerArgs,
		"-v", fmt.Sprintf("%s:/inspect:ro", inspectorPath),
		"--entrypoint", "/inspect",
		image)

	// Add inspector arguments
	if args.Pattern != "" {
		dockerArgs = append(dockerArgs, "--glob", args.Pattern)
	}
	if args.MD5 {
		dockerArgs = append(dockerArgs, "--md5")
	}
	if args.NoTimes {
		dockerArgs = append(dockerArgs, "--no-times")
	}
	if args.Path != "/" {
		dockerArgs = append(dockerArgs, "--path", args.Path)
	}
	if args.OutputDir != "" {
		dockerArgs = append(dockerArgs, "--output-dir", "/inspect-target")
		dockerArgs = append(dockerArgs, "--strip-components", fmt.Sprintf("%d", args.StripComponents))
		if args.PreserveOwner {
			dockerArgs = append(dockerArgs, "--preserve-owner")
		}
		if args.PreservePermissions {
			dockerArgs = append(dockerArgs, "--preserve-perms")
		}
	}
	// Create a pipe for capturing stdout while also displaying it
	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stderr = os.Stderr
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	return output, err
	/*
		// This is a version that lets us debug what the docker command is printing
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout pipe: %v", err)
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("failed to start inspection: %v", err)
		}

		// Create a buffer to store the JSON output
		var output []byte
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				output = append(output, buf[:n]...)
				os.Stdout.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to read output: %v", err)
			}
		}

		// Wait for the command to complete
		if err := cmd.Wait(); err != nil {
			return nil, fmt.Errorf("inspection failed: %v", err)
		}
		return output, nil
	*/
}

func main() {
	var args Args
	// Set defaults
	args.Summary = false
	args.Path = "/"

	arg.MustParse(&args)

	if args.PreserveAll {
		args.PreserveOwner = true
		args.PreservePermissions = true
	}
	// check if we actually can handle the owner preservation
	if runtime.GOOS == "darwin" && args.OutputDir != "" && args.PreserveOwner {
		if !isOwnershipSupported(args.OutputDir) {
			fmt.Fprintf(os.Stderr, "filesystem of %q does not support ownership changes\n", args.OutputDir)
			os.Exit(1)
		}
	}

	// Run inspection on first image
	files1JSON, err := runInspector(args.Image1, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Inspection failed: %v\n", err)
		os.Exit(1)
	}

	if args.Image2 != "" {
		// Parse the JSON outputs
		var files1 []FileInfo
		if err := json.Unmarshal(files1JSON, &files1); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse inspection results: %v", err)
			os.Exit(1)
		}

		// Run inspection on second image
		files2JSON, err := runInspector(args.Image2, args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Second inspection failed: %v\n", err)
			os.Exit(1)
		}
		var files2 []FileInfo
		if err := json.Unmarshal(files2JSON, &files2); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse inspection results: %v", err)
			os.Exit(1)
		}

		// Compare the results
		mode := CompareAll
		if args.NoTimes {
			mode = CompareNoTimes
		}

		result, err := Compare(files1, files2, mode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error comparing images: %v\n", err)
			os.Exit(1)
		}

		// Output the comparison results
		if args.JSON {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			encoder.Encode(result)
		} else {
			printDiffText(result)
		}

		// Exit with status 1 if differences were found
		if result.Summary.TotalDifferences > 0 {
			os.Exit(1)
		}
	} else {
		var files1 []FileInfo
		if args.JSON {
			// we just print what we got
			fmt.Print(string(files1JSON))
		} else {
			if err := json.Unmarshal(files1JSON, &files1); err != nil {
				fmt.Fprintf(os.Stderr, "failed to parse inspection results: %v", err)
				os.Exit(1)
			}
			// Output the inspection results
			var totalSize int64
			dirCount := 0
			fileCount := 0
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
			header := "Mode\tSize\tModified\tUser\tGroup\tPath\tSymlink"
			if args.MD5 {
				header += "\tMD5"
			}
			fmt.Fprintln(w, header)

			for _, file := range files1 {
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
				if file.IsDir {
					dirCount++
				} else {
					fileCount++
				}
				totalSize += file.Size

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

			// Print summary if requested
			if args.Summary {
				fmt.Printf("\nSummary:\n")
				fmt.Printf("Total size: %d bytes\n", totalSize)
				fmt.Printf("Directories: %d\n", dirCount)
				fmt.Printf("Files: %d\n", fileCount)
			}
		}

		// If we're on macOS and files were copied with ownership preservation requested,
		// fix ownership using sudo
		if runtime.GOOS == "darwin" && args.OutputDir != "" &&
			args.PreserveOwner {
			// Test if ownership changes are supported
			if args.JSON {
				if err := json.Unmarshal(files1JSON, &files1); err != nil {
					fmt.Fprintf(os.Stderr, "failed to parse inspection results: %v", err)
					os.Exit(1)
				}
			}
			fmt.Fprintf(os.Stderr, "\nFixing file ownership on macOS...")
			if err := fixOwnershipWithSudo(files1, args.OutputDir, args.StripComponents); err != nil {
				fmt.Fprintf(os.Stderr, "\nError fixing ownership: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, " Done!\n")
		}
	}
}

// In main.go, modify the ownership fixing:
func fixOwnershipWithSudo(files []FileInfo, outputDir string, stripComponents int) error {
	// Build a script of chown commands
	var commands strings.Builder
	commands.WriteString("#!/bin/bash\n")

	for _, file := range files {
		// Get the adjusted path based on strip components
		destPath := getDestPath(file.Path, stripComponents)
		if destPath == "" {
			continue
		}

		// Extract UID/GID from the user/group strings
		uid, err := extractID(file.User)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not extract UID from %q: %v\n", file.User, err)
			continue
		}
		gid, err := extractID(file.Group)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not extract GID from %q: %v\n", file.Group, err)
			continue
		}

		fullDestPath := filepath.Join(outputDir, destPath)
		// Use -h to handle symlinks correctly
		fmt.Fprintf(&commands, "chown -h %d:%d %q\n", uid, gid, fullDestPath)
	}

	// Create a temporary script file
	scriptFile, err := os.CreateTemp("", "docker-inspector-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create script file: %v", err)
	}
	defer os.Remove(scriptFile.Name())

	if err := os.WriteFile(scriptFile.Name(), []byte(commands.String()), 0700); err != nil {
		return fmt.Errorf("failed to write script: %v", err)
	}

	//fmt.Println(commands.String())
	// Run the script with sudo
	cmd := exec.Command("sudo", "/bin/bash", scriptFile.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to fix ownership: %v", err)
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

func extractID(s string) (int, error) {
	// Find the last pair of parentheses
	openIdx := strings.LastIndex(s, "(")
	closeIdx := strings.LastIndex(s, ")")
	if openIdx == -1 || closeIdx == -1 || openIdx >= closeIdx {
		return 0, fmt.Errorf("no ID found in %q", s)
	}

	// Extract and parse the ID
	idStr := s[openIdx+1 : closeIdx]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("invalid ID in %q: %v", s, err)
	}
	return id, nil
}
