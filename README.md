# Docker Image Inspector

A command-line tool to inspect the contents of Docker images without having to manually create containers or extract tar files. The tool creates a temporary container, inspects its filesystem, and cleans up automatically.

## Beware: Experimental (WIP)

This is work in progress and not finished or bug free. In fact there are known problems and everything was only tested on osx so far.

## Features

- Cross-platform: Runs on macOS, Linux, and Windows (with Linux containers)
- Inspects any Docker image without modifying it
- Extracts files from images to local filesystem
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
# Inspect a single image
./docker-inspector nginx:latest

# Compare two images
./docker-inspector nginx:latest nginx:1.24
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

# Extract files from image
./docker-inspector nginx:latest --output-dir ./extracted --glob "**/*.conf"

# Extract with preserved permissions and ownership
./docker-inspector nginx:latest --output-dir ./extracted --preserve-all

# Extract stripping leading path components
./docker-inspector nginx:latest --output-dir ./extracted --glob "/etc/nginx/**" --strip-components 2
```

### Image Comparison Mode

When two images are specified, the tool operates in comparison mode, showing the differences between them:

```bash
# Compare two different versions
./docker-inspector nginx:latest nginx:1.24

# Compare with content verification
./docker-inspector nginx:latest nginx:1.24 --md5

# Focus on specific files
./docker-inspector nginx:latest nginx:1.24 --glob "**/*.conf"

# Compare without modification times
./docker-inspector nginx:latest nginx:1.24 --no-times

# Get machine-readable diff
./docker-inspector nginx:latest nginx:1.24 --json
```

The comparison shows:
- Added files (present in second image but not in first)
- Removed files (present in first image but not in second)
- Modified files with details about what changed:
  - Size differences
  - Permission changes
  - Ownership changes
  - Content changes (when --md5 is used)
  - Modification time changes (unless --no-times is specified)

Example output:
```
Comparison Summary:
Total differences: 5
Added files: 2
Removed files: 1
Modified files: 2

Details:
+ /etc/nginx/new-feature.conf
  (1234 bytes, nginx:nginx, mode -rw-r--r--)
- /etc/nginx/deprecated.conf
  (890 bytes, root:root, mode -rw-r--r--)
M /etc/nginx/nginx.conf
  size changed: 1500 -> 1600
  content changed (different MD5)
M /etc/nginx/conf.d/default.conf
  permissions changed: -rw-r--r-- -> -rw-r--r--
```

The tool exits with:
- Status 0 if no differences are found
- Status 1 if differences are found or an error occurs

This is useful for:
- Validating image updates
- Auditing configuration changes
- Checking for unwanted modifications
- Automation and CI/CD pipelines


### File Extraction Options

The tool can extract files from Docker images to your local filesystem:

- `--output-dir <path>`: Extract matching files to this directory
- `--preserve-permissions`: Preserve file permissions when extracting
- `--preserve-user`: Preserve user/group ownership when extracting (requires root/sudo)
- `--preserve-all`: Preserve all file attributes (equivalent to both above)
- `--strip-components N`: Strip N leading components from file names when extracting

For example, with `--strip-components 2`, a file path `/etc/nginx/nginx.conf` becomes `nginx.conf` in the output directory.

Note: When preserving ownership on macOS:
- Docker Desktop's implementation limits ownership preservation through bind mounts
- The destination filesystem must support Unix ownership attributes
- The tool will automatically use sudo to fix ownership after the copy
- Some macOS volumes (like external drives) might not support ownership changes

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
--path               Path inside the container to inspect (default: "/")
--json              Output in JSON format
--summary           Show summary statistics
--glob              Glob pattern for matching files (supports **/)
--md5               Calculate MD5 checksums for files
--keep              Keep the temporary container after inspection
--no-times          Exclude modification times from output (useful for diffs)
--output-dir        Extract matching files to this directory
--strip-components  Strip NUMBER leading components from file names when extracting
--preserve-perms    Preserve file permissions when extracting
--preserve-owner    Preserve user/group ownership when extracting
--preserve-all      Preserve all file attributes
```

## How It Works

The tool:
1. Creates a temporary container from the specified image
2. Copies a specialized Linux inspector binary into the container
3. Executes the inspector inside the container
4. Collects and formats the results
5. When extracting files:
   - Mounts the output directory into the container
   - Copies files with requested attributes preserved
   - On macOS, uses sudo to fix ownership if requested
6. Automatically cleans up the container (unless --keep is specified)

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