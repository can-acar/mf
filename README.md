# mf

A cross-platform, Go-based alternative to `rimraf` for removing files and directories with ease.

## Features

- Fast and reliable file/directory removal
- Safe recursive deletes
- Multi-platform support (Windows, macOS, Linux)
- Simple CLI usage

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
