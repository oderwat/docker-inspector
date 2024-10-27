package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
)

//go:embed internal-inspector
var internalInspector []byte

type Args struct {
	Image   string `arg:"positional,required" help:"docker image to inspect"`
	Path    string `arg:"--path" default:"/" help:"path inside the container to inspect"`
	JSON    bool   `arg:"--json" help:"output in JSON format"`
	Summary bool   `arg:"--summary" help:"show summary statistics"`
	Pattern string `arg:"--glob" help:"glob pattern for matching files (supports **/)"`
	MD5     bool   `arg:"--md5" help:"calculate MD5 checksums for files"`
	Keep    bool   `arg:"--keep" help:"keep the temporary container after inspection"`
	NoTimes bool   `arg:"--no-times" help:"exclude modification times from output"`
}

func (Args) Version() string {
	return "docker-inspector 1.0.0"
}

func (Args) Description() string {
	return "Docker image content inspector - examines files and directories inside a container image"
}

func runInspector(containerID string, args Args) error {
	// Create a temporary directory for the inspector
	tempDir, err := os.MkdirTemp("", "docker-inspector-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Write the embedded Linux inspector to the temp directory
	inspectorPath := filepath.Join(tempDir, "internal-inspector")
	if err := os.WriteFile(inspectorPath, internalInspector, 0755); err != nil {
		return fmt.Errorf("failed to write inspector: %v", err)
	}

	// Copy the inspector to the container
	copyCmd := exec.Command("docker", "cp", inspectorPath, fmt.Sprintf("%s:/inspect", containerID))
	if output, err := copyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to copy inspector to container: %v\n%s", err, output)
	}

	// Build the command arguments
	dockerArgs := []string{"exec", containerID, "/inspect"}
	if args.JSON {
		dockerArgs = append(dockerArgs, "--json")
	}
	if args.Summary {
		dockerArgs = append(dockerArgs, "--summary")
	}
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

	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	var args Args
	// Set defaults
	args.Summary = false
	args.Path = "/"

	arg.MustParse(&args)

	// First, ensure the image exists or can be pulled
	pullCmd := exec.Command("docker", "pull", args.Image)
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to pull image %s: %v\n", args.Image, err)
		os.Exit(1)
	}

	// Start a temporary container
	startCmd := exec.Command("docker", "run", "-d", "--entrypoint", "sleep", args.Image, "3600")
	output, err := startCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start container: %v\n%s\n", err, output)
		os.Exit(1)
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		fmt.Fprintf(os.Stderr, "Failed to get container ID\n")
		os.Exit(1)
	}

	// Give the container a moment to start
	time.Sleep(1 * time.Second)

	// Ensure container cleanup unless --keep is specified
	if !args.Keep {
		defer func() {
			stopCmd := exec.Command("docker", "rm", "-f", containerID)
			stopCmd.Run()
		}()
	}

	// Run the inspection
	if err := runInspector(containerID, args); err != nil {
		fmt.Fprintf(os.Stderr, "Inspection failed: %v\n", err)
		os.Exit(1)
	}

	if args.Keep {
		fmt.Printf("\nContainer ID: %s\n", containerID)
	}
}
