---
title: Installation
description: How to install and set up Ard on various platforms.
---

## Prerequisites

Ard requires Go 1.26 or later to build and run. Ensure Go is installed on the system:

```bash
go version
```

## Downloading Pre-built Binaries (Linux/macOS)

Pre-built binaries for Unix-based systems are available on the [releases page](https://github.com/akonwi/ard/releases). Download the binary for your operating system.

Extract the binary and add it to your PATH, or move it to a directory already in your PATH like `/usr/local/bin/`.

## Installing from Source

Currently, Ard is available only from source. Clone the repository and build:

```bash
git clone https://github.com/akonwi/ard.git
cd ard/compiler
go mod download
go build --tags=goexperiment.jsonv2 -o ard main.go
```

This creates an `ard` executable in the `compiler` directory.

## Add to PATH

To use Ard from anywhere, add the executable to the system PATH:

## Verify Installation

```bash
ard version
```

## Next Steps

[Create your first program](/getting-started/first-program/).
