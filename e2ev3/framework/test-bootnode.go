package framework

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

const localhost string = "127.0.0.1"

type TestBootnode struct {
	t             *testing.T
	clusterConfig *TestClusterConfig
	node          *node
	logs          bytes.Buffer
	enode         string
	port          string
}

func NewBootnode(t *testing.T, clusterConfig *TestClusterConfig, port int64) *TestBootnode {
	bootnode := &TestBootnode{
		t:             t,
		clusterConfig: clusterConfig,
		port:          fmt.Sprint(port),
	}
	bootnode.Start()
	return bootnode
}

func (t *TestBootnode) Start() {
	// Build arguments
	args := []string{
		"bootnode",
		"--listen-addr", localhost,
		"--port", t.port,
	}

	stdout := t.clusterConfig.GetStdout(t.clusterConfig.Name)
	writer := io.MultiWriter(&t.logs, stdout)

	node, err := newNode(t.clusterConfig.Binary, args, writer)
	if err != nil {
		t.t.Error(err)
	}

	enode, err := t.getEnode(10*time.Second, 100*time.Millisecond)
	if err != nil {
		t.t.Error(err)
	}

	t.enode = enode
	t.node = node
}

func (t *TestBootnode) Stop() {
	if err := t.node.Stop(); err != nil {
		t.t.Error(err)
	}
	t.node = nil
}

// getEnode function returns enode of the started Boot node
// blocks for a first run until it timeouts
func (t *TestBootnode) getEnode(timeout, period time.Duration) (string, error) {
	if len(t.enode) > 0 {
		return t.enode, nil
	}

	timer := time.NewTimer(timeout)
	ticker := time.NewTicker(period)
	// enode appears as a first output line from cmd
	for {
		select {
		case <-timer.C:
			return "", errors.New("get enode timeout")
		case <-ticker.C:
			logs := t.logs.String()
			startIdx := strings.Index(logs, "enode://")
			if startIdx != -1 {
				enode := logs[startIdx:]
				endIdx := strings.IndexAny(enode, " \n\r")
				if endIdx != -1 {
					enode = enode[:endIdx]
				}

				return enode, nil
			}
		}
	}
}
