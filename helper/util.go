package helper

func GetData(data []byte, start uint64, size uint64) []byte {
	length := uint64(len(data))
	if start > length {
		start = length
	}
	end := start + size
	if end > length {
		end = length
	}
	return RightPadBytes(data[start:end], int(size))
}

func RightPadBytes(b []byte, size int) []byte {
	l := len(b)
	if l > size {
		return b
	}

	tmp := make([]byte, size)
	copy(tmp[0:], b)
	return tmp
}

func LeftPadBytes(b []byte, size int) []byte {
	l := len(b)
	if l > size {
		return b
	}

	tmp := make([]byte, size)
	copy(tmp[size-l:], b)
	return tmp
}
