package polybft

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

type service struct {
	proto.UnimplementedPolybftServer

	srv *Polybft
}

func (s *service) ConsensusSnapshot(
	ctx context.Context,
	req *proto.ConsensusSnapshotRequest) (*proto.ConsensusSnapshotResponse, error) {

	latestBlock := s.srv.blockchain.CurrentHeader()

	accounts, err := s.srv.validatorsCache.GetSnapshot(latestBlock.Number, nil)
	if err != nil {
		return nil, err
	}

	resp := &proto.ConsensusSnapshotResponse{
		Validators:  []*proto.Validator{},
		BlockNumber: latestBlock.Number,
	}

	for _, acct := range accounts {
		resp.Validators = append(resp.Validators, &proto.Validator{
			Address:     acct.Address.String(),
			VotingPower: acct.VotingPower,
		})
	}

	return resp, nil
}

func (s *service) Bridge(ctx context.Context, req *proto.BridgeRequest) (*proto.BridgeResponse, error) {
	header := s.srv.blockchain.CurrentHeader()

	provider, err := s.srv.blockchain.GetStateProviderForBlock(header)
	if err != nil {
		return nil, err
	}

	state := s.srv.blockchain.GetSystemState(s.srv.consensusConfig, provider)

	epoch, err := state.GetEpoch()
	if err != nil {
		return nil, err
	}

	indx, err := s.srv.state.lastBundleIndex()
	if err != nil {
		return nil, err
	}

	nextCommittedIndex, err := state.GetNextCommittedIndex()
	if err != nil {
		return nil, err
	}

	nextExecutionIndex, err := state.GetNextExecutionIndex()
	if err != nil {
		return nil, err
	}

	stateSyncs, err := s.srv.state.list()
	if err != nil {
		return nil, err
	}

	fmt.Println("-- state syncs --")
	fmt.Println(stateSyncs)

	resp := &proto.BridgeResponse{
		Epoch:              epoch,
		NextCommittedIndex: nextCommittedIndex,
		NextExecutionIndex: nextExecutionIndex,
		StateSyncs:         []*proto.StateSync{},
		LastCommittedIndex: indx,
	}

	for _, c := range stateSyncs {
		resp.StateSyncs = append(resp.StateSyncs, &proto.StateSync{
			Id:     c.ID,
			Sender: c.Sender.String(),
			Target: c.Receiver.String(),
		})
	}

	return resp, nil
}

func (s *service) BridgeCall(ctx context.Context, req *proto.BridgeCallRequest) (*proto.BridgeCallResponse, error) {
	stateSyncID := req.Index

	bundlesToExecute, err := s.srv.state.getBundles(stateSyncID, 1)
	if err != nil {
		return nil, fmt.Errorf("cannot get bundles: %w", err)
	}

	if len(bundlesToExecute) == 0 {
		return nil, fmt.Errorf("cannot find bundle containing StateSync id %d", stateSyncID)
	}

	for _, bundle := range bundlesToExecute[0].StateSyncs {
		if bundle.ID == stateSyncID {
			stateSyncProof := &types.StateSyncProof{
				Proof:     bundlesToExecute[0].Proof,
				StateSync: types.StateSyncEvent(*bundle),
			}

			input, err := types.ExecuteBundleABIMethod.Encode([2]interface{}{stateSyncProof.Proof, stateSyncEventsToAbiSlice2(stateSyncProof.StateSync)})
			if err != nil {
				return nil, err
			}

			resp := &proto.BridgeCallResponse{
				Data: hex.EncodeToString(input),
				StateSync: &proto.StateSync{
					Id:     bundle.ID,
					Sender: bundle.Sender.String(),
					Target: bundle.Receiver.Address().String(),
				},
			}
			return resp, nil
		}
	}

	return nil, fmt.Errorf("failed")
}

func stateSyncEventsToAbiSlice2(stateSyncEvent types.StateSyncEvent) []map[string]interface{} {
	result := make([]map[string]interface{}, 1)
	result[0] = map[string]interface{}{
		"id":       stateSyncEvent.ID,
		"sender":   stateSyncEvent.Sender,
		"receiver": stateSyncEvent.Receiver,
		"data":     stateSyncEvent.Data,
		"skip":     stateSyncEvent.Skip,
	}

	return result
}
