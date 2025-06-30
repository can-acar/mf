package main

import (
	"runtime"
	"strings"
)

func IsDangerousPath(path string) bool {
	if runtime.GOOS == "windows" {
		norm := strings.ToLower(path)
		if norm == "c:\\" || norm == "c:\\windows" || norm == "c:\\program files" || norm == "c:\\users" {
			return true
		}
	} else {
		if path == "/" || path == "/etc" || path == "/bin" || path == "/usr" || path == "/lib" {
			return true
		}
	}
	return false
}
