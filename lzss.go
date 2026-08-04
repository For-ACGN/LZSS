package lzss

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// about search windows size
const (
	DefaultWindowSize = 1024
	MaximumWindowSize = 4096
)

// when chain length is MaximumChainLen, it will use brute-force search
// for the best compression, but it is slowest, otherwise, it will use
// N-candidate hash chain for trade-off between speed and compression.
const (
	MinimumChainLen = 1
	MaximumChainLen = 16
)

const (
	minMatchLength = 3
	maxMatchLength = 18

	// hash table size: 2^12 = 4096 buckets for 3-byte hash.
	hashBits = 12
	hashSize = 1 << hashBits
)

// Compress is used to compress data using LZSS with configurable parameters.
//
// Parameters:
//
//	windowSize: sliding window size (1-4096, 0 for default 1024)
//	chainLen: hash chain length for match search
//	  1 = single hash candidate (fastest, worst compression)
//	  N = N-candidate hash chain (trade-off between speed and compression)
//	 16 = brute-force (bytes.Index, the best compression, slowest)
//
// Typical recommendations:
//
//	chainLen=1:  fastest          (~36%), ~139 MB/s
//	chainLen=6:  good balance     (~40%), ~52 MB/s
//	chainLen=16: best compression (~42%), ~3.7 MB/s
func Compress(data []byte, windowSize, chainLen int) ([]byte, error) {
	if windowSize < 0 || windowSize > MaximumWindowSize {
		return nil, errors.New("invalid window size")
	}
	if chainLen < MinimumChainLen || chainLen > MaximumChainLen {
		return nil, errors.New("invalid chain length")
	}
	if windowSize == 0 {
		windowSize = DefaultWindowSize
	}
	switch chainLen {
	case MinimumChainLen:
		return compressWithSingleHashCandidate(data, windowSize), nil
	case MaximumChainLen:
		return compressWithBruteForce(data, windowSize), nil
	default:
		return compressWithNHashCandidate(data, windowSize, chainLen), nil
	}
}

func compressWithSingleHashCandidate(data []byte, windowSize int) []byte {
	// initialize hash table
	hashTable := make([]uint16, hashSize)
	var (
		flag    byte
		flagPtr int
		flagCtr int
	)
	dataPtr := 0
	dataLen := len(data)
	output := make([]byte, len(data)+len(data)/8+2)
	outPtr := 1
	for dataPtr < dataLen {
		rem := dataLen - dataPtr
		var (
			offset int
			length int
		)
		if rem >= minMatchLength {
			h := hash3(data[dataPtr:])
			stored := hashTable[h]
			if stored != 0 {
				candidate := resolveCandidate(stored, dataPtr)
				dist := dataPtr - candidate
				if dist > 0 && dist <= windowSize {
					maxLen := rem
					if maxLen > maxMatchLength {
						maxLen = maxMatchLength
					}
					matchLen := 0
					for matchLen < maxLen && data[candidate+matchLen] == data[dataPtr+matchLen] {
						matchLen++
					}
					if matchLen >= minMatchLength {
						offset = dist - 1
						length = matchLen
					}
				}
			}
		}
		// set compress flag and write data
		if length != 0 {
			flag |= 1
			mark := uint16(offset<<4 | (length - minMatchLength))
			output[outPtr+0] = byte(mark)
			output[outPtr+1] = byte(mark >> 8)
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
		// advance
		advance := 1
		if length != 0 {
			advance = length
		}
		// update hash
		for i := 0; i < advance && dataPtr+i+2 < dataLen; i++ {
			h := hash3(data[dataPtr+i:])
			hashTable[h] = uint16(dataPtr + i + 1)
		}
		dataPtr += advance
	}
	// process the final flag block
	if flagCtr != 0 {
		flag <<= byte(7 - flagCtr)
		output[flagPtr] = flag
	} else {
		outPtr--
	}
	return output[:outPtr]
}

func compressWithNHashCandidate(data []byte, windowSize, chainLen int) []byte {
	// initialize hash table
	hashTable := make([]uint16, hashSize*chainLen)
	var (
		flag    byte
		flagPtr int
		flagCtr int
	)
	dataPtr := 0
	dataLen := len(data)
	output := make([]byte, len(data)+len(data)/8+2)
	outPtr := 1
	for dataPtr < dataLen {
		rem := dataLen - dataPtr
		var (
			offset int
			length int
		)
		if rem >= minMatchLength {
			h := hash3(data[dataPtr:])
			base := int(h) * chainLen
			maxLen := rem
			if maxLen > maxMatchLength {
				maxLen = maxMatchLength
			}
			// search hash chain
			for i := 0; i < chainLen; i++ {
				stored := hashTable[base+i]
				if stored == 0 {
					break
				}
				candidate := resolveCandidate(stored, dataPtr)
				dist := dataPtr - candidate
				if dist <= 0 || dist > windowSize {
					continue
				}
				matchLen := 0
				for matchLen < maxLen && data[candidate+matchLen] == data[dataPtr+matchLen] {
					matchLen++
				}
				if matchLen >= minMatchLength && matchLen > length {
					offset = dist - 1
					length = matchLen
					if matchLen == maxLen {
						break
					}
				}
			}
		}
		// set compress flag and write data
		if length != 0 {
			flag |= 1
			mark := uint16(offset<<4 | (length - minMatchLength))
			output[outPtr+0] = byte(mark)
			output[outPtr+1] = byte(mark >> 8)
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
		// advance
		advance := 1
		if length != 0 {
			advance = length
		}
		// update hash chain
		for i := 0; i < advance && dataPtr+i+2 < dataLen; i++ {
			h := hash3(data[dataPtr+i:])
			base := int(h) * chainLen
			for j := chainLen - 1; j > 0; j-- {
				hashTable[base+j] = hashTable[base+j-1]
			}
			hashTable[base] = uint16(dataPtr + i + 1)
		}
		dataPtr += advance
	}
	// process the final flag block
	if flagCtr != 0 {
		flag <<= byte(7 - flagCtr)
		output[flagPtr] = flag
	} else {
		outPtr--
	}
	return output[:outPtr]
}

func compressWithBruteForce(data []byte, windowSize int) []byte {
	var (
		window  []byte
		flag    byte
		flagPtr int
		flagCtr int
	)
	dataPtr := 0
	dataLen := len(data)
	output := make([]byte, len(data)+len(data)/8+2)
	outPtr := 1
	for dataPtr < dataLen {
		rem := dataLen - dataPtr
		// search the same data in current window
		var (
			offset int
			length int
		)
		if rem >= minMatchLength {
			// scan the window once, finding all 3-byte prefix matches
			// and extending each to find the best (longest then nearest) match
			sub := data[dataPtr : dataPtr+minMatchLength]
			maxLen := rem
			if maxLen > maxMatchLength {
				maxLen = maxMatchLength
			}
			bestOffset := 0
			bestLength := 0
			pos := 0
			for pos <= len(window)-minMatchLength {
				idx := bytes.Index(window[pos:], sub)
				if idx == -1 {
					break
				}
				absPos := pos + idx
				// extend the match
				matchLen := minMatchLength
				for matchLen < maxLen && absPos+matchLen < len(window) &&
					window[absPos+matchLen] == data[dataPtr+matchLen] {
					matchLen++
				}
				newOffset := len(window) - absPos - 1
				// prefer longer matches; equal length → prefer nearer (smaller offset)
				if matchLen > bestLength || (matchLen == bestLength && newOffset < bestOffset) {
					bestLength = matchLen
					bestOffset = newOffset
					if matchLen == maxLen {
						break
					}
				}
				pos = absPos + 1
			}
			if bestLength >= minMatchLength {
				offset = bestOffset
				length = bestLength
			}
		}
		// set compress flag and write data
		if length != 0 {
			flag |= 1
			// 12 bit = offset, 4 bit = length
			// offset max is 4095, max length value is [0-15] + 3
			mark := uint16(offset<<4 | (length - minMatchLength))
			output[outPtr+0] = byte(mark)
			output[outPtr+1] = byte(mark >> 8)
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
	return output[:outPtr]
}

// hash3 computes a 12-bit hash of 3 consecutive bytes.
// reference Multiply-Shift Hash.
func hash3(b []byte) uint32 {
	v := uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
	return (v * 0x1E35A7BD) >> (32 - hashBits)
}

// resolveCandidate reconstructs an absolute data position from a uint16 stored value.
// The stored value is (position + 1) as uint16, with 0 as the sentinel for "empty".
// Since the window is at most 4096 bytes, only one 64KB block can contain the
// candidate (-1 if same block, previous block otherwise).
func resolveCandidate(stored uint16, dataPtr int) int {
	lo := int(stored) - 1
	candidate := (dataPtr & ^0xFFFF) | lo
	if candidate > dataPtr {
		candidate -= 1 << 16
	}
	return candidate
}

// Decompress is used to decompress LZSS compressed data.
func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	output := make([]byte, 0, len(data)*2)
	var flag [8]bool
	flagIdx := 8
	dataPtr := 0
	dataLen := len(data)
	for dataPtr < dataLen {
		// Read flag block when needed.
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
			if dataPtr+1 >= dataLen {
				return nil, errors.New("truncated match reference")
			}
			mark := binary.LittleEndian.Uint16(data[dataPtr:])
			offset := int(mark>>4 + 1)
			length := int(mark&0xF + minMatchLength)
			start := len(output) - offset
			if start < 0 {
				return nil, errors.New("invalid match offset")
			}
			// copy length bytes, handling overlapping matches correctly.
			for i := 0; i < length; i++ {
				output = append(output, output[start+i])
			}
			dataPtr += 2
		} else {
			if dataPtr >= dataLen {
				return nil, errors.New("truncated literal")
			}
			output = append(output, data[dataPtr])
			dataPtr++
		}
		flagIdx++
	}
	return output, nil
}
