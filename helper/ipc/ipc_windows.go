//go:build windows
// +build windows

package ipc

import (
	"net"
	"time"

	"gopkg.in/natefinch/npipe.v2"
)

// Dial dials an IPC path
func Dial(path string) (net.Conn, error) {
	return npipe.Dial(path)
}

// DialTimeout dials an IPC path with timeout
func DialTimeout(path string, timeout time.Duration) (net.Conn, error) {
	return npipe.DialTimeout(path, timeout)
}

// Listen listens an IPC path
func Listen(path string) (net.Listener, error) {
	return npipe.Listen(path)
}
