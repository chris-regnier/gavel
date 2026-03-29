# Installation

## Download Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/chris-regnier/gavel/releases).

```bash
# macOS (arm64)
curl -L https://github.com/chris-regnier/gavel/releases/latest/download/gavel_<version>_Darwin_arm64.tar.gz | tar xz
sudo mv gavel_Darwin_arm64 /usr/local/bin/gavel

# Linux (amd64)
curl -L https://github.com/chris-regnier/gavel/releases/latest/download/gavel_<version>_Linux_x86_64.tar.gz | tar xz
sudo mv gavel_Linux_x86_64 /usr/local/bin/gavel

# Windows (amd64)
# Download the .zip file from the releases page and extract
```

## Build from Source

### Prerequisites

- Go 1.24+
- [Task](https://taskfile.dev/) (task runner)
- [BAML CLI](https://docs.boundaryml.com/) (for regenerating the LLM client)
- An LLM provider (see [Providers](PROVIDERS.md))

```bash
git clone https://github.com/chris-regnier/gavel.git
cd gavel
task build
```

This produces a `gavel` binary in the `dist/` directory.
