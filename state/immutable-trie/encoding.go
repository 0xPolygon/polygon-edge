package itrie

// hasTerminator checks if hex is ending
// with a terminator flag.
func hasTerminator(hex []byte) bool {
	if len(hex) == 0 {
		return false
	}

	return hex[len(hex)-1] == 16
}

// encodeCompact packs a hex sequence (of nibbles)
// into compact encoding.
func encodeCompact(hex []byte) []byte {
	var terminator int

	if hasTerminator(hex) {
		// remove terminator flag
		hex = hex[:len(hex)-1]
		terminator = 1
	} else {
		terminator = 0
	}

	// determine prefix flag
	oddLen := len(hex) % 2
	flag := 2*terminator + oddLen

	// insert flag
	if oddLen == 1 {
		hex = append([]byte{byte(flag)}, hex...)
	} else {
		hex = append([]byte{byte(flag), byte(0)}, hex...)
	}

	// hex slice is of even length now - pack nibbles
	result := make([]byte, len(hex)/2)
	for i := 0; i < cap(result); i++ {
		result[i] = hex[2*i]<<4 | hex[2*i+1]
	}

	return result
}

// bytesToHexNibbles splits bytes into nibbles
// (with terminator flag). Prefix flag is not removed.
func bytesToHexNibbles(bytes []byte) []byte {
	nibbles := make([]byte, len(bytes)*2+1)
	for i, b := range bytes {
		nibbles[i*2] = b / 16
		nibbles[i*2+1] = b % 16
	}

	nibbles[len(nibbles)-1] = 16

	return nibbles
}

// decodeCompact unpacks compact encoding
// into a hex sequence of nibbles.
func decodeCompact(compact []byte) []byte {
	base := bytesToHexNibbles(compact)

	// remove the terminator flag
	if base[0] < 2 {
		base = base[:len(base)-1]
	}

	// remove prefix flag
	if base[0]&1 == 1 {
		// odd length
		base = base[1:]
	} else {
		// even length
		base = base[2:]
	}

	return base
}
