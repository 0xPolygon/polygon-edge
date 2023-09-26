package eventtracker

import (
	"encoding/json"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

type getNewStateF struct {
	Address               types.Address
	Number                uint64
	LastProcessed         uint64
	BatchSize             uint64
	NumBlockConfirmations uint64
	MaxBackLogSize        uint64
}

func FuzzGetNewState(f *testing.F) {
	seeds := []getNewStateF{
		{
			Address:               types.Address(types.StringToAddress("1").Bytes()),
			Number:                25,
			LastProcessed:         9,
			BatchSize:             5,
			NumBlockConfirmations: 3,
			MaxBackLogSize:        1000,
		},
		{
			Address:               types.Address(types.StringToAddress("1").Bytes()),
			Number:                30,
			LastProcessed:         29,
			BatchSize:             5,
			NumBlockConfirmations: 3,
			MaxBackLogSize:        1000,
		},
		{
			Address:               types.Address(types.StringToAddress("2").Bytes()),
			Number:                100,
			LastProcessed:         10,
			BatchSize:             10,
			NumBlockConfirmations: 3,
			MaxBackLogSize:        15,
		},
	}

	for _, seed := range seeds {
		data, err := json.Marshal(seed)
		if err != nil {
			return
		}

		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		var data getNewStateF
		if err := json.Unmarshal(input, &data); err != nil {
			t.Skip(err)
		}

		providerMock := new(mockProvider)
		for blockNum := data.LastProcessed + 1; blockNum <= data.Number; blockNum++ {
			providerMock.On("GetBlockByNumber", ethgo.BlockNumber(blockNum), false).Return(&ethgo.Block{Number: blockNum}, nil).Once()
		}

		logs := []*ethgo.Log{
			createTestLogForStateSyncEvent(t, 1, 1),
			createTestLogForStateSyncEvent(t, 1, 11),
			createTestLogForStateSyncEvent(t, 2, 3),
		}
		providerMock.On("GetLogs", mock.Anything).Return(logs, nil)

		testConfig := createTestTrackerConfig(t, data.NumBlockConfirmations, data.BatchSize, data.MaxBackLogSize)
		testConfig.BlockProvider = providerMock

		eventTracker := &EventTracker{
			config:         testConfig,
			blockContainer: NewTrackerBlockContainer(data.LastProcessed),
		}

		require.NoError(t, eventTracker.getNewState(&ethgo.Block{Number: data.Number}))
	})
}
