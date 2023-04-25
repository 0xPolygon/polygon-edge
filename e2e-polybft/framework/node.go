package framework

import (
	"io"
	"os"
	"os/exec"
	"sync/atomic"
)

type node struct {
	shuttingDown atomic.Bool
	cmd          *exec.Cmd
	doneCh       chan struct{}
	exitResult   *exitResult
}

func newNode(binary string, args []string, stdout io.Writer) (*node, error) {
	cmd := exec.Command(binary, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stdout

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	n := &node{
		cmd:    cmd,
		doneCh: make(chan struct{}),
	}
	go n.run()

	return n, nil
}

func (n *node) ExitResult() *exitResult {
	return n.exitResult
}

func (n *node) Wait() <-chan struct{} {
	return n.doneCh
}

func (n *node) run() {
	err := n.cmd.Wait()

	n.exitResult = &exitResult{
		Signaled: n.IsShuttingDown(),
		Err:      err,
	}
	close(n.doneCh)
	n.cmd = nil
}

func (n *node) IsShuttingDown() bool {
	return n.shuttingDown.Load()
}

func (n *node) Stop() error {
	if n.cmd == nil {
		// the server is already stopped
		return nil
	}

	if err := n.cmd.Process.Signal(os.Interrupt); err != nil {
		return err
	}

	n.shuttingDown.Store(true)
	<-n.Wait()

	return nil
}

type exitResult struct {
	Signaled bool
	Err      error
}
