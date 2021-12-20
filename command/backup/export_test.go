package backup

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/server/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
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
	for _, b := range blocks {
		b.Header.ComputeHash()
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
			select {
			case res := <-processExportStream(tt.mockSystemExportClient, &buffer):
				assert.Equal(t, tt.err, res.err)
				if res.err != nil {
					return
				}

				assert.Equal(t, tt.from, *res.from)
				assert.Equal(t, tt.to, *res.to)

				// create expected data
				expectedData := make([]byte, 0)
				for _, rv := range tt.mockSystemExportClient.recvs {
					if rv.err != nil {
						break
					}
					expectedData = append(expectedData, rv.event.Data...)
				}
				assert.Equal(t, expectedData, buffer.Bytes())
			case <-time.After(5 * time.Second):
				t.Fatal(errors.New("timeout"))
			}

		})
	}
}
