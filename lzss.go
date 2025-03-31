package lzss

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const (
	minMatchLength = 3
	maxMatchLength = 18

	defaultWindowSize = 1024
	maximumWindowSize = 4096
)

// Compress is used to compress the raw data with window size.
func Compress(data []byte, windowSize int) ([]byte, error) {
	if windowSize > maximumWindowSize || windowSize < 0 {
		return nil, errors.New("invalid window size")
	}
	if windowSize == 0 {
		windowSize = defaultWindowSize
	}
	var (
		window  []byte
		flag    byte
		flagPtr int
		flagCtr int
	)
	dataPtr := 0
	dataLen := len(data)
	output := make([]byte, len(data)*9/8+1)
	outPtr := 1
	// for encode offset+length
	buf := make([]byte, 2)
	for dataPtr < dataLen {
		rem := dataLen - dataPtr
		// search the same data in current window
		var (
			offset int
			length int
		)
		for l := minMatchLength; l <= maxMatchLength; l++ {
			if rem < l {
				break
			}
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
		if flagCtr == 7 {
			output[flagPtr] = flag
			// update pointer
			flagPtr = outPtr
			outPtr++
			// reset status
			flag = 0
			flagCtr = 0
		} else {
			flag <<= 1
			flagCtr++
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
	// process the final flag block
	if flagCtr != 0 {
		flag <<= byte(7 - flagCtr)
		output[flagPtr] = flag
	} else {
		outPtr-- // rollback pointer
	}
	return output[:outPtr], nil
}

// Decompress is used to decompress the compressed data.
func Decompress(data []byte) []byte {
	var flag [8]bool
	flagIdx := 8
	output := bytes.Buffer{}
	outPtr := 0
	dataPtr := 0
	dataLen := len(data)
	for dataPtr < dataLen {
		// check need read flag block
		if flagIdx == 8 {
			b := data[dataPtr]
			flag[0] = (b & (1 << 7)) != 0
			flag[1] = (b & (1 << 6)) != 0
			flag[2] = (b & (1 << 5)) != 0
			flag[3] = (b & (1 << 4)) != 0
			flag[4] = (b & (1 << 3)) != 0
			flag[5] = (b & (1 << 2)) != 0
			flag[6] = (b & (1 << 1)) != 0
			flag[7] = (b & (1 << 0)) != 0
			dataPtr++
			flagIdx = 0
		}
		if flag[flagIdx] {
			mark := binary.LittleEndian.Uint16(data[dataPtr:])
			offset := int(mark>>4 + 1)
			length := int(mark&0xF + minMatchLength)
			start := outPtr - offset
			block := output.Bytes()[start : start+length]
			output.Write(block)
			dataPtr += 2
			outPtr += length
		} else {
			output.WriteByte(data[dataPtr])
			dataPtr++
			outPtr++
		}
		// update flag index
		flagIdx++
	}
	return output.Bytes()
}
