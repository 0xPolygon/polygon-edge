package itrie

import (
	"encoding/binary"
	"fmt"
)

// Optimized RLP decoding library based on fastjson. It will likely replace minimal/rlp in the future.
// Note, The content marshalled may not be correct marshalled again. For now, that is not an intended use.

type Parser struct {
	// b contains working copy of the string to be parsed.
	b []byte

	// c is a cache for json values.
	c cache
}

func (p *Parser) Parse(b []byte) (*Value, error) {
	p.b = append(p.b[:0], b...)
	p.c.reset()

	v, _, err := parseValue(p.b, &p.c)
	if err != nil {
		return nil, fmt.Errorf("cannot parse RLP: %s", err)
	}
	return v, nil
}

func parseValue(b []byte, c *cache) (*Value, []byte, error) {
	if len(b) == 0 {
		return nil, b, fmt.Errorf("cannot parse empty string")
	}

	cur := b[0]
	if cur < 0x80 {
		v := c.getValue()
		v.t = TypeBytes
		v.b = []byte{cur}
		return v, b[1:], nil
	}
	if cur < 0xB8 {
		v, tail, err := parseBytes(b[1:], int(cur-0x80), c)
		if err != nil {
			return nil, tail, fmt.Errorf("cannot parse short bytes: %s", err)
		}
		return v, tail, nil
	}
	if cur < 0xC0 {
		size, tail, err := readUint(b[1:], int(cur-0xB7))
		if err != nil {
			return nil, tail, fmt.Errorf("cannot read size of long bytes: %s", err)
		}
		v, tail, err := parseBytes(tail, size, c)
		if err != nil {
			return nil, tail, fmt.Errorf("cannot parse long bytes: %s", err)
		}
		return v, tail, nil
	}
	if cur < 0xF8 {
		v, tail, err := parseList(b[1:], int(cur-0xC0), c)
		if err != nil {
			return nil, tail, fmt.Errorf("cannot parse short bytes: %s", err)
		}
		return v, tail, nil
	}

	// long array
	size, tail, err := readUint(b[1:], int(cur-0xf7))
	if err != nil {
		return nil, tail, fmt.Errorf("cannot read size of long array: %s", err)
	}
	v, tail, err := parseList(tail, size, c)
	if err != nil {
		return nil, tail, fmt.Errorf("cannot parse long array: %s", err)
	}
	return v, tail, nil
}

func parseBytes(b []byte, size int, c *cache) (*Value, []byte, error) {
	if size > len(b) {
		return nil, nil, fmt.Errorf("length is not enough")
	}

	v := c.getValue()
	v.t = TypeBytes
	v.b = b[:size]
	v.l = size
	return v, b[size:], nil
}

func parseList(b []byte, size int, c *cache) (*Value, []byte, error) {
	a := c.getValue()
	a.t = TypeArray
	a.a = a.a[:0]

	for size > 0 {
		var v *Value
		var err error

		pre := len(b)
		v, b, err = parseValue(b, c)
		if err != nil {
			return nil, b, fmt.Errorf("cannot parse array value: %s", err)
		}

		a.a = append(a.a, v)
		size -= pre - len(b)
	}
	if size < 0 {
		return nil, nil, fmt.Errorf("bad ending")
	}
	return a, b, nil
}

func readUint(b []byte, size int) (int, []byte, error) {
	buf := make([]byte, 8)
	copy(buf[8-size:], b[:size])
	i := binary.BigEndian.Uint64(buf)
	return int(i), b[size:], nil
}
