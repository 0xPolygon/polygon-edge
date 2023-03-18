package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	serverProto "github.com/0xPolygon/polygon-edge/server/proto"
	txpoolProto "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Tests if interceptor triggers validation of grpc requests
func TestE2E_GRPCRequestValidationTriggering(t *testing.T) {
	const (
		validatorCount = 5
		testTimeout    = time.Second * 30
	)

	// create cluster
	cluster := framework.NewTestCluster(t, validatorCount,
		framework.WithValidatorSnapshot(validatorCount))
	defer cluster.Stop()

	ctx := context.Background()
	err := cluster.WaitUntil(testTimeout, 2*time.Second, func() bool {
		peerList, err := cluster.Servers[0].Conn().PeersList(ctx, &emptypb.Empty{})

		return err == nil && len(peerList.GetPeers()) > 0
	})
	require.NoError(t, err)

	// SystemService
	// call PeersStatus with invalid data
	_, err = cluster.Servers[0].Conn().PeersStatus(ctx, &serverProto.PeersStatusRequest{Id: "-1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "request validation failed: invalid PeersStatusRequest.Id: value does not match regex pattern")

	//TxnPoolOperatorService
	// call AddTxn with invalid data
	_, err = cluster.Servers[0].TxnPoolOperator().AddTxn(ctx, &txpoolProto.AddTxnReq{Raw: nil, From: "12"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "request validation failed: invalid AddTxnReq.Raw: value is required; invalid AddTxnReq.From: value does not match regex pattern")
}
