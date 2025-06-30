//go:build !windows

package main

import "context"

type LockFinder struct{}
type ProcessInfo struct {
	PID  int32
	Name string
}

func NewLockFinder() *LockFinder {
	return &LockFinder{}
}

func (lf *LockFinder) FindLockingProcesses(ctx context.Context, path string) ([]ProcessInfo, error) {
	return nil, nil
}

func (lf *LockFinder) KillProcess(proc ProcessInfo) error {
	return nil
}