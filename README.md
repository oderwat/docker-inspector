# Docker Image Inspector

A command-line tool to inspect the contents of Docker images without having to manually create containers or extract tar files. The tool creates a temporary container, inspects its filesystem, and cleans up automatically.

## Features

- Cross-platform: Runs on macOS, Linux, and Windows (with Linux containers)
- Inspects any Docker image without modifying it
- Recursive directory listing
- Glob pattern support (including `**/`) for finding specific files
- MD5 checksum calculation for files
- JSON output option for automated processing
- Detailed summaries of files, directories, and sizes
- Clean handling of special filesystems (/proc, /sys, etc.)
- Modification time handling for reliable diffs

## Installation

1. Clone the repository
2. Build for your platform:
```bash
# For macOS
make darwin

# For Linux
make linux

# For Windows
make windows
```

## Usage

Basic usage:
```bash
./docker-inspector nginx:latest
```

With options:
```bash
# Find specific files
./docker-inspector nginx:latest --glob "**/*.conf"

# Calculate MD5 checksums
./docker-inspector nginx:latest --md5

# Output as JSON
./docker-inspector nginx:latest --json > nginx-files.json

# Inspect specific path
./docker-inspector nginx:latest --path /etc/nginx

# Keep container for further inspection
./docker-inspector nginx:latest --keep
```

### Comparing Images

To compare the contents of two images or two runs of the same image:

```bash
# With modification times (might show differences due to container startup)
./docker-inspector nginx:latest --json --md5 > run1.txt
./docker-inspector nginx:latest --json --md5 > run2.txt
diff run1.txt run2.txt

# Without modification times (more reliable for structural comparisons)
./docker-inspector nginx:latest --json --md5 --no-times > run1.txt
./docker-inspector nginx:latest --json --md5 --no-times > run2.txt
diff run1.txt run2.txt
```

Note: Files like /etc/resolv.conf typically show modification time differences between runs due to container startup configuration. Using --md5 helps identify actual content changes regardless of timestamps.

### Options

```
--path           Path inside the container to inspect (default: "/")
--json          Output in JSON format
--summary       Show summary statistics
--glob          Glob pattern for matching files (supports **/)
--md5           Calculate MD5 checksums for files
--keep          Keep the temporary container after inspection
--no-times      Exclude modification times from output (useful for diffs)
```

## How It Works

The tool:
1. Creates a temporary container from the specified image
2. Copies a specialized Linux inspector binary into the container
3. Executes the inspector inside the container
4. Collects and formats the results
5. Automatically cleans up the container (unless --keep is specified)

## Building from Source

Requires:
- Go 1.21 or later
- Docker running with Linux containers
- make

```bash
# Build for current platform
make

# Or for specific platform
make darwin   # For macOS
make linux    # For Linux
make windows  # For Windows
```

## Credits

Most of the implementation work was done by Claude (Anthropic) in a conversation about Docker image inspection requirements and cross-platform Go development. The original concept and requirements were provided by the repository owner.

## License

[MIT-License](LICENSE.txt)