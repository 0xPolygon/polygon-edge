//go:build !windows
// +build !windows

package ipc

import (
	"net"
	"os"
	"path/filepath"
	"time"
)

// Dial dials an IPC path
func Dial(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}

// DialTimeout dials an IPC path with timeout
func DialTimeout(path string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", path, timeout)
}

// Listen listens an IPC path
func Listen(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0751); err != nil {
		return nil, err
	}

	if removeErr := os.Remove(path); removeErr != nil {
		return nil, removeErr
	}

	lis, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	if chmodErr := os.Chmod(path, 0600); chmodErr != nil {
		return nil, chmodErr
	}

	return lis, nil
}
