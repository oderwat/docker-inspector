package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/alexflint/go-arg"
	"os"
	"os/exec"
	"path/filepath"
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
}

func (Args) Version() string {
	return "docker-inspector 1.1.0"
}

func (Args) Description() string {
	return "Docker image content inspector - examines and compares files inside container images"
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

	// Create a pipe for capturing stdout while also displaying it
	cmd := exec.Command("docker", dockerArgs...)
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
		if args.JSON {
			// we just print what we got
			fmt.Print(string(files1JSON))
		} else {
			var files1 []FileInfo
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
	}
}
