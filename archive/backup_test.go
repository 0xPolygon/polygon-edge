package archive

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/0xPolygon/polygon-edge/server/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type recvData struct {
	event *proto.ExportEvent
	err   error
}

type mockSystemExportClient struct {
	proto.System_ExportClient
	recvs []recvData
	cur   int
}

func (m *mockSystemExportClient) Recv() (*proto.ExportEvent, error) {
	if m.cur >= len(m.recvs) {
		return nil, io.EOF
	}

	recv := m.recvs[m.cur]
	m.cur++

	return recv.event, recv.err
}

var (
	genesis = &types.Block{
		Header: &types.Header{
			Hash:   types.StringToHash("genesis"),
			Number: 0,
		},
	}
	blocks = []*types.Block{
		{
			Header: &types.Header{
				Number: 1,
			},
		},
		{
			Header: &types.Header{
				Number: 2,
			},
		},
		{
			Header: &types.Header{
				Number: 3,
			},
		},
	}
)

func init() {
	genesis.Header.ComputeHash()

	for _, b := range blocks {
		b.Header.ComputeHash()
	}
}

type systemClientMock struct {
	proto.SystemClient
	status       *proto.ServerStatus
	errForStatus error
	block        *proto.BlockResponse
	errForBlock  error
}

func (m *systemClientMock) GetStatus(context.Context, *emptypb.Empty, ...grpc.CallOption) (*proto.ServerStatus, error) {
	return m.status, m.errForStatus
}

func (m *systemClientMock) BlockByNumber(
	context.Context,
	*proto.BlockByNumberRequest, ...grpc.CallOption,
) (*proto.BlockResponse, error) {
	return m.block, m.errForBlock
}

func Test_determineTo(t *testing.T) {
	t.Parallel()

	toPtr := func(x uint64) *uint64 {
		return &x
	}

	tests := []struct {
		name string
		// input
		targetTo *uint64
		// mock
		systemClientMock proto.SystemClient
		// result
		resTo     uint64
		resToHash types.Hash
		err       error
	}{
		{
			name:     "should return expected 'to'",
			targetTo: toPtr(2),
			systemClientMock: &systemClientMock{
				status: &proto.ServerStatus{
					Current: &proto.ServerStatus_Block{
						// greater than targetTo
						Number: 10,
					},
				},
				block: &proto.BlockResponse{
					Data: blocks[1].MarshalRLP(),
				},
			},
			resTo:     2,
			resToHash: blocks[1].Hash(),
			err:       nil,
		},
		{
			name:     "should return latest if target to is greater than the latest in node",
			targetTo: toPtr(2),
			systemClientMock: &systemClientMock{
				status: &proto.ServerStatus{
					Current: &proto.ServerStatus_Block{
						// less than targetTo
						Number: 1,
						Hash:   blocks[1].Hash().String(),
					},
				},
			},
			resTo:     1,
			resToHash: blocks[1].Hash(),
			err:       nil,
		},
		{
			name:     "should fail if GetStatus failed",
			targetTo: toPtr(2),
			systemClientMock: &systemClientMock{
				status:       nil,
				errForStatus: errors.New("fake error"),
			},
			resTo:     0,
			resToHash: types.Hash{},
			err:       errors.New("fake error"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resTo, resToHash, err := determineTo(context.Background(), tt.systemClientMock, tt.targetTo)
			assert.Equal(t, tt.err, err)
			if tt.err == nil {
				assert.Equal(t, tt.resTo, resTo)
				assert.Equal(t, tt.resToHash, resToHash)
			}
		})
	}
}

func Test_processExportStream(t *testing.T) {
	tests := []struct {
		name                   string
		mockSystemExportClient *mockSystemExportClient
		// result
		from uint64
		to   uint64
		err  error
	}{
		{
			name: "should be succeed with event",
			mockSystemExportClient: &mockSystemExportClient{
				recvs: []recvData{
					{
						event: &proto.ExportEvent{
							From: 1,
							To:   2,
							Data: append(blocks[0].MarshalRLP(), blocks[1].MarshalRLP()...),
						},
					},
				},
			},
			from: 1,
			to:   2,
			err:  nil,
		},
		{
			name: "should succeed with multiple events",
			mockSystemExportClient: &mockSystemExportClient{
				recvs: []recvData{
					{
						event: &proto.ExportEvent{
							From: 1,
							To:   2,
							Data: append(blocks[0].MarshalRLP(), blocks[1].MarshalRLP()...),
						},
					},
					{
						event: &proto.ExportEvent{
							From: 3,
							To:   3,
							Data: blocks[2].MarshalRLP(),
						},
					},
				},
			},
			from: 1,
			to:   3,
			err:  nil,
		},
		{
			name: "should fail when received error",
			mockSystemExportClient: &mockSystemExportClient{
				recvs: []recvData{
					{
						event: &proto.ExportEvent{
							From: 1,
							To:   2,
							Data: append(blocks[0].MarshalRLP(), blocks[1].MarshalRLP()...),
						},
					},
					{
						err: errors.New("failed to send"),
					},
					{
						event: &proto.ExportEvent{
							From: 3,
							To:   3,
							Data: blocks[2].MarshalRLP(),
						},
					},
				},
			},
			from: 0,
			to:   0,
			err:  errors.New("failed to send"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buffer bytes.Buffer
			from, to, err := processExportStream(tt.mockSystemExportClient, hclog.NewNullLogger(), &buffer, 0, 0)

			assert.Equal(t, tt.err, err)
			if err != nil {
				return
			}

			assert.Equal(t, tt.from, *from)
			assert.Equal(t, tt.to, *to)

			// create expected data
			expectedData := make([]byte, 0)
			for _, rv := range tt.mockSystemExportClient.recvs {
				if rv.err != nil {
					break
				}
				expectedData = append(expectedData, rv.event.Data...)
			}
			assert.Equal(t, expectedData, buffer.Bytes())
		})
	}
}
