package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/golang/protobuf/ptypes/empty"
)

func TestPOW(t *testing.T) {
	srv := framework.NewTestServer(t, func(config *framework.TestServerConfig) {
		config.Seal = true
	})
	defer srv.Stop()

	clt := srv.Operator()

	// wait enough to advance the pow chain
	time.Sleep(10 * time.Second)

	status, err := clt.GetStatus(context.Background(), &empty.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if status.Current.Number != 0 {
		t.Fatal("it has not advanced")
	}
}
