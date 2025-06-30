# mf

A cross-platform, Go-based alternative to `rimraf` for removing files and directories with ease.

## Features

 MF is a sophisticated, cross-platform file deletion tool with advanced capabilities:
Core Features:
  - Recursive directory deletion (rimraf alternative)
  - Dry run mode for safe previewing
  - Pattern-based exclusion system
  - Comprehensive asynchronous logging

  Advanced Features (Windows):
  - Process lock detection and termination
  - System-critical process protection
  - Admin privilege handling
  - Performance optimizations with object pooling

  Architecture:
  - Worker pool pattern for concurrency
  - Platform-specific implementations via build tags
  - Defensive programming with extensive safety checks
  - Context-based cancellation and timeout handling

  The tool is designed for handling stubborn locked files, particularly in Windows environments where processes
  commonly prevent file deletion.

## Installation

### Pre-built Binary

Download the latest release from [Releases](https://github.com/can-acar/mf/releases) and add it to your PATH.

### Build from Source

Make sure you have [Go](https://golang.org/dl/) installed.

```sh
git clone https://github.com/can-acar/mf.git
cd mf
go build -o mf
```
# Windows CMD ailas
```sh
doskey mf=<mf-build-path>
doskey mf=mf.exe
```

# PowerShell alias
```sh
Set-Alias mfdel "C:\mf.exe"
Set-Alias gs git

notepad $PROFILE

paste :
Set-Alias mf "C:\mf.exe"
Set-Alias gs git
Set-Alias ga git
