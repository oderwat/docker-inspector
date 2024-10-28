# Docker Image Inspector

A command-line tool for inspecting, comparing, and extracting files from Docker images without having to manually create containers. The tool creates a temporary container, inspects its filesystem, and cleans up automatically. It can list files and their attributes, compare two images to find differences, and extract files from images to the local filesystem.

## Disclaimer

This is WIP and "use it on your own risk". There are known and unknown bugs!

## Features

- Cross-platform: Runs on macOS, Linux, and Windows (with Linux containers)
- Three main modes of operation:
  - Inspect: List files and their attributes in any Docker image
  - Compare: Show detailed differences between two Docker images
  - Extract: Copy files from Docker images to local filesystem
- Recursive directory listing
- Glob pattern support (including `**/`) for finding specific files
- MD5 checksum calculation for files
- JSON output option for automated processing
- Detailed summaries of files, directories, and sizes
- Clean handling of special filesystems (/proc, /sys, etc.)
- Modification time handling for reliable diffs
- Preserves file permissions and ownership during extraction

## Installation (precompiled binary)

Download an executable for your architecture from [Release Page](https://github.com/oderwat/docker-inspector/releases/latest)

Extract the ZIP and copy it to you path.

## Installation (from source)

- Clone the repository
- Build for your platform:
```bash
make
```

To follow the examples you need to copy the binary into your path or use it with the correct name of the executable (e.g. `./docker-inspector.exe`) 

## Build for other platforms (cross compilation)

```bash
# For Mac
make darwin
#./docker-inspector-darwin

# For Linux
make linux
#./docker-inspector-linux

# For Windows (cross compilation)
make windows
#./docker-inspector.exe
```

## Usage

Basic usage:
```bash
# Inspect a single image
docker-inspector nginx:latest

# Compare two images
docker-inspector nginx:latest nginx:1.24
```

With options:
```bash
# Find specific files
docker-inspector nginx:latest --glob "**/*.conf"

# Calculate MD5 checksums
docker-inspector nginx:latest --md5

# Output as JSON
docker-inspector nginx:latest --json > nginx-files.json

# Inspect specific path
docker-inspector nginx:latest --path /etc/nginx

# Keep container for further inspection
docker-inspector nginx:latest --keep

# Extract files from image
docker-inspector nginx:latest --output-dir ./extracted --glob "**/*.conf"

# Extract with preserved permissions and ownership
docker-inspector nginx:latest --output-dir ./extracted --preserve-all

# Extract stripping leading path components
docker-inspector nginx:latest --output-dir ./extracted --glob "/etc/nginx/**" --strip-components 2
```

### Comparing Images

The tool can directly compare two Docker images to show their differences:

```bash
# Direct comparison of two images
docker-inspector nginx:latest nginx:1.24

# Compare with content verification
docker-inspector nginx:latest nginx:1.24 --md5

# Focus on specific files
docker-inspector nginx:latest nginx:1.24 --glob "**/*.conf"

# Compare without modification times
docker-inspector nginx:latest nginx:1.24 --no-times

# Get machine-readable comparison
docker-inspector nginx:latest nginx:1.24 --json
```

Alternatively, you can generate and compare JSON outputs manually:
```bash
# Generate JSONs separately and use external diff tools
docker-inspector nginx:latest --json --md5 > image1.json
docker-inspector nginx:1.24 --json --md5 > image2.json
diff image1.json image2.json
# or use jq, etc.
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

### Options

`docker-inspector --help`

```
Docker image content inspector - examines, extracts and compares files inside container images
docker-inspector 1.1.0
Usage: docker-inspector-darwin [--path PATH] [--json] [--summary] [--glob GLOB] [--md5] [--keep] [--no-times] [--output-dir OUTPUT-DIR] [--strip-components STRIP-COMPONENTS] [--preserve-owner] [--preserve-perms] [--preserve-all] IMAGE1 [IMAGE2]

Positional arguments:
  IMAGE1                 docker image to inspect (or first image when comparing)
  IMAGE2                 second docker image (for comparison mode)

Options:
  --path PATH            path inside the container to inspect [default: /]
  --json                 output in JSON format
  --summary              show summary statistics
  --glob GLOB            glob pattern for matching files (supports **/)
  --md5                  calculate MD5 checksums for files
  --keep                 keep the temporary container after inspection
  --no-times             exclude modification times from output
  --output-dir OUTPUT-DIR
                         extract matching files to this directory
  --strip-components STRIP-COMPONENTS
                         strip NUMBER leading components from file names
  --preserve-owner       preserve user/group information when extracting
  --preserve-perms       preserve file permissions when extracting
  --preserve-all         preserve all file attributes
  --help, -h             display this help and exit
  --version              display version and exit
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

## Known bugs

Directories are not handled well so far:

- Extraction (using `--output-dir`) is not creating empty directories
- `--preserver-owner` is not working for directories either

Cutting of path elements using `--strip-components` is sketchy in this implementation.

## Caveats

- `--glob` is applied to the full path, so you need `/etc/**` to get all files and directory from /etc and `/etc/*` to get just the files.
- Preserving permissions on OSX needs a sketchy implementation that uses `sudo` with a temporary bash script.
- OSX external APF drives are usually not preserving ownership (this is why you can share them between macs with different user ids)

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

Most of the implementation work was done using Claude (Anthropic) in a conversation about Docker image inspection requirements and cross-platform Go development. The original concept and requirements were provided by the repository owner.

## License

[MIT-License](LICENSE.txt)