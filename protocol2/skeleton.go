package protocol2

import (
	"fmt"

	"github.com/0xPolygon/minimal/types"
)

type skeleton struct {
	slots []*slot
}

func (s *skeleton) addSkeleton(headers []*types.Header) error {
	// safe check make sure they all have the same difference
	diff := uint64(0)
	for i := 1; i < len(headers); i++ {
		elemDiff := headers[i].Number - headers[i-1].Number
		if diff == 0 {
			diff = elemDiff
		} else if elemDiff != diff {
			return fmt.Errorf("bad diff")
		}
	}

	// fill up the slots
	s.slots = make([]*slot, len(headers))
	for _, header := range headers {
		slot := &slot{
			hash:   header.Hash,
			number: header.Number,
			blocks: make([]*types.Block, diff),
		}
		s.slots = append(s.slots, slot)
	}
	return nil
}

type slot struct {
	hash      types.Hash
	number    uint64
	blocks    []*types.Block
	completed bool
}
