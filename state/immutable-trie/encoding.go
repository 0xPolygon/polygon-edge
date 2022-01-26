package itrie

// hasTerm checks if hex is ending
// with a terminator flag.
func hasTerm(hex []byte) bool {
	if len(hex) == 0 {
		return false
	}

	return hex[len(hex)-1] == 16
}

func hexToCompact(hex []byte) []byte {
	// check terminator flag
	var terminator int
	if hasTerm(hex) {
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

func keybytesToHex(str []byte) []byte {
	nibbles := make([]byte, len(str)*2+1)
	for i, b := range str {
		nibbles[i*2] = b / 16
		nibbles[i*2+1] = b % 16
	}

	nibbles[len(nibbles)-1] = 16

	return nibbles
}

func compactToHex(compact []byte) []byte {
	base := keybytesToHex(compact)
	// delete terminator flag
	if base[0] < 2 {
		base = base[:len(base)-1]
	}
	// apply odd flag
	if base[0]&1 == 1 {
		base = base[1:]
	} else {
		base = base[2:]
	}

	return base
}
