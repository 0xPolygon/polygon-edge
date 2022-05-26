// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package runtime

const (
	set2BitsMask = uint16(0b11)
	set3BitsMask = uint16(0b111)
	set4BitsMask = uint16(0b1111)
	set5BitsMask = uint16(0b1_1111)
	set6BitsMask = uint16(0b11_1111)
	set7BitsMask = uint16(0b111_1111)
)

// bitvec is a bit vector which maps bytes in a program.
// An unset bit means the byte is an opcode, a set bit means
// it's data (i.e. argument of PUSHxx).
type bitvec []byte

func (bits bitvec) set1(pos uint64) {
	bits[pos/8] |= 1 << (pos % 8)
}

func (bits bitvec) setN(flag uint16, pos uint64) {
	a := flag << (pos % 8)
	bits[pos/8] |= byte(a)
	if b := byte(a >> 8); b != 0 {
		bits[pos/8+1] = b
	}
}

func (bits bitvec) set8(pos uint64) {
	a := byte(0xFF << (pos % 8))
	bits[pos/8] |= a
	bits[pos/8+1] = ^a
}

func (bits bitvec) set16(pos uint64) {
	a := byte(0xFF << (pos % 8))
	bits[pos/8] |= a
	bits[pos/8+1] = 0xFF
	bits[pos/8+2] = ^a
}
