package lzss

import (
	"bytes"
	"encoding/binary"
)

const (
	minMatchLength = 3
	maxMatchLength = 18
	windowSize     = 2048
)

// Compress is used to compress raw data.
func Compress(data []byte) []byte {
	var (
		window  []byte
		dataPtr int
		flag    byte
		flagPtr int
		fCtr    int
	)
	dataLen := len(data)
	output := make([]byte, len(data)*9/8)
	outPtr := 1
	// for encode offset+length
	buf := make([]byte, 2)
	for dataPtr < dataLen {
		rem := dataLen - dataPtr
		if rem < maxMatchLength {
			output[flagPtr] = flag
			block := data[dataPtr:]
			copy(output[outPtr:], block)
			outPtr += len(block)
			break
		}
		// search the same data in current window
		var (
			offset int
			length int
		)
		for l := minMatchLength; l <= maxMatchLength; l++ {
			idx := bytes.Index(window, data[dataPtr:dataPtr+l])
			if idx == -1 {
				break
			}
			offset = len(window) - idx - 1
			length = l
		}
		// set compress flag and write data
		if length != 0 {
			flag |= 1
			// 12 bit = offset, 4 bit = length
			// offset max is 4095, max length value is [0-15] + 3
			mark := uint16(offset<<4 + (length - minMatchLength))
			binary.LittleEndian.PutUint16(buf, mark)
			copy(output[outPtr:], buf)
			outPtr += 2
		} else {
			output[outPtr] = data[dataPtr]
			outPtr++
		}
		// update flag block
		if fCtr == 7 {
			output[flagPtr] = flag
			// update pointer
			flagPtr = outPtr
			outPtr++
			// reset status
			flag = 0
			fCtr = 0
		} else {
			flag <<= 1
			fCtr++
		}
		// update data pointer
		if length != 0 {
			dataPtr += length
		} else {
			dataPtr++
		}
		// update window
		start := dataPtr - windowSize
		if start < 0 {
			start = 0
		}
		window = data[start:dataPtr]
	}
	return output[:outPtr]
}
