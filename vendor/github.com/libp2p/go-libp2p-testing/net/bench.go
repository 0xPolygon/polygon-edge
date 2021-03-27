package tnet

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/atomic"
	"google.golang.org/grpc/benchmark/latency"
)

// WriteTrackedConn provides a wrapper for tracking how many write calls are made to a network connection.
type WriteTrackedConn struct {
	net.Conn
	WriteCount atomic.Uint32
}

func (c *WriteTrackedConn) Write(b []byte) (n int, err error) {
	c.WriteCount.Inc()
	return c.Conn.Write(b)
}

// NetworkTestFunc is a benchmark function under test by `FindNetworkLimit`
type NetworkTestFunc func(b *testing.B, n1, n2 net.Conn)

// ConnectionForNetwork generates a pair of network connections with a specified latency.
func ConnectionForNetwork(n *latency.Network) (n1, n2 *WriteTrackedConn, err error) {
	var wg sync.WaitGroup
	wg.Add(1)

	var listener net.Listener
	listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return
	}
	slowListener := n.Listener(listener)
	go func() {
		defer wg.Done()
		ac, _ := slowListener.Accept()
		slowListener.Close()
		n2 = &WriteTrackedConn{ac, atomic.Uint32{}}
		return
	}()
	baseDialer := net.Dialer{}
	dialer := n.ContextDialer(baseDialer.DialContext)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dialer(ctx, "tcp4", slowListener.Addr().String())
	if err != nil {
		return
	}
	n1 = &WriteTrackedConn{conn, atomic.Uint32{}}

	wg.Wait()
	return
}

func benchWithNet(testFunc NetworkTestFunc, n *latency.Network) func(*testing.B) {
	return func(sb *testing.B) {
		n1, n2, err := ConnectionForNetwork(n)
		if err != nil {
			sb.Error(err)
		}
		defer n1.Close()
		defer n2.Close()
		testFunc(sb, n1, n2)
		sb.ReportMetric(float64(n1.WriteCount.Load()), "writes")
	}
}

// FindNetworkLimit benchmarks a function to analyze CPU and network relationship
func FindNetworkLimit(testFunc NetworkTestFunc) (int, error) {
	network := latency.Network{
		Kbps:    0,
		Latency: 0,
	}

	wrapperFunc := benchWithNet(testFunc, &network)

	result := testing.Benchmark(wrapperFunc)
	if result.N < 1 {
		return 0, fmt.Errorf("failed to run benchmark")
	}
	fmt.Printf("Limit with no network latency: %s\n", result)

	last := 1.0
	network.Latency = 30 * time.Millisecond
	network.Kbps = 1024 * 1 // 1 Mbps
	result = testing.Benchmark(wrapperFunc)
	current := (float64(result.Bytes) * float64(result.N) / 1e6) / result.T.Seconds()
	for current/(2*last) > .1 {
		network.Latency *= 2
		result = testing.Benchmark(wrapperFunc)
		last = current
		current = (float64(result.Bytes) * float64(result.N) / 1e6) / result.T.Seconds()
	}
	fmt.Printf("At 30ms latency, bandwidth can saturate: %dKbps\n", network.Kbps)

	return network.Kbps, nil
}

// ParallelismSlowdown tracks how much overhead is incurred on a ntework bound function when parallelism contentention
// in increased.
func ParallelismSlowdown(testFunc NetworkTestFunc, kbps int, l time.Duration) (slowdown float64, err error) {
	maxProcs := runtime.GOMAXPROCS(0)
	runtime.GOMAXPROCS(maxProcs)
	if maxProcs > 1 {
		runtime.GOMAXPROCS(1)
	}

	network := latency.Network{
		Kbps:    kbps,
		Latency: l,
	}
	prevBytes := int64(-1)
	ratio := 1.0
	for i := 1; i <= maxProcs; i *= 2 {
		runtime.GOMAXPROCS(i)
		result := testing.Benchmark(benchWithNet(testFunc, &network))
		if prevBytes > 0 {
			ratio = float64(result.Bytes) / float64(prevBytes)
		}
		prevBytes = result.Bytes
		fmt.Printf("At MaxProc %d %dKbps / %s latency: %s\n", i, network.Kbps, network.Latency, result)
	}
	fmt.Printf("Slowdown is %f%%\n", 100*(1.0-ratio))
	return (1.0 - ratio), nil
}
