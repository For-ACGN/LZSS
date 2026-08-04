package lzss

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const (
	minMatchLength    = 3
	maxMatchLength    = 18
	defaultWindowSize = 1024
	maximumWindowSize = 4096

	// hash table size: 2^12 = 4096 buckets for 3-byte hash.
	hashBits = 12
	hashSize = 1 << hashBits
)

// hash3 computes a 12-bit hash of 3 consecutive bytes.
func hash3(b []byte) uint32 {
	return (uint32(b[0])<<10 ^ uint32(b[1])<<5 ^ uint32(b[2])) & (hashSize - 1)
}

// Compress compresses data using LZSS with default settings (windowSize=1024, brute-force search).
func Compress(data []byte, windowSize int) ([]byte, error) {
	return CompressWith(data, windowSize, 0)
}

// CompressWith compresses data using LZSS with configurable parameters.
//
// Parameters:
//   - windowSize: sliding window size (1-4096, 0 for default 1024)
//   - chainLen: hash chain length for match search
//     0 = brute-force (bytes.Index, the best compression, slowest)
//     1 = single hash candidate (fastest, worst compression)
//     N = N-candidate hash chain (trade-off between speed and compression)
//
// Typical recommendations:
//   - chainLen=0: best compression (~48%), ~7 MB/s
//   - chainLen=6: good balance     (~44%), ~52 MB/s
//   - chainLen=1: fastest          (~36%), ~93 MB/s
func CompressWith(data []byte, windowSize, chainLen int) ([]byte, error) {
	if windowSize > maximumWindowSize || windowSize < 0 {
		return nil, errors.New("lzss: invalid window size")
	}
	if windowSize == 0 {
		windowSize = defaultWindowSize
	}
	if chainLen < 0 {
		return nil, errors.New("lzss: invalid chain length")
	}

	// Initialize hash table for chain-based search.
	var hashTable [][]int
	if chainLen > 0 {
		hashTable = make([][]int, hashSize)
		for i := range hashTable {
			hashTable[i] = make([]int, chainLen)
			for j := range hashTable[i] {
				hashTable[i][j] = -1
			}
		}
	}

	var (
		window  []byte // only used by brute-force mode
		flag    byte
		flagPtr int
		flagCtr int
	)
	dataPtr := 0
	dataLen := len(data)
	output := make([]byte, len(data)*9/8+1)
	outPtr := 1
	buf := make([]byte, 2)

	for dataPtr < dataLen {
		rem := dataLen - dataPtr
		var (
			offset int
			length int
		)

		if chainLen > 0 {
			// Hash chain search.
			if rem >= minMatchLength {
				h := hash3(data[dataPtr:])
				maxLen := maxMatchLength
				if rem < maxLen {
					maxLen = rem
				}
				for _, candidate := range hashTable[h] {
					if candidate < 0 {
						break
					}
					dist := dataPtr - candidate
					if dist <= 0 || dist > windowSize {
						continue
					}
					matchLen := 0
					for matchLen < maxLen && data[candidate+matchLen] == data[dataPtr+matchLen] {
						matchLen++
					}
					if matchLen >= minMatchLength && matchLen > length {
						offset = dist
						length = matchLen
						if length == maxLen {
							break
						}
					}
				}
			}
		} else {
			// Brute-force search (original algorithm).
			for l := minMatchLength; l <= maxMatchLength; l++ {
				if rem < l {
					break
				}
				idx := bytes.Index(window, data[dataPtr:dataPtr+l])
				if idx == -1 {
					break
				}
				offset = len(window) - idx
				length = l
			}
		}

		// Encode match or literal.
		if length != 0 {
			flag |= 1
			mark := uint16((offset-1)<<4 + (length - minMatchLength))
			binary.LittleEndian.PutUint16(buf, mark)
			copy(output[outPtr:], buf)
			outPtr += 2
		} else {
			output[outPtr] = data[dataPtr]
			outPtr++
		}

		// Update flag block.
		if flagCtr == 7 {
			output[flagPtr] = flag
			flagPtr = outPtr
			outPtr++
			flag = 0
			flagCtr = 0
		} else {
			flag <<= 1
			flagCtr++
		}

		// Advance.
		advance := 1
		if length != 0 {
			advance = length
		}

		if chainLen > 0 {
			// Update hash chain.
			for i := 0; i < advance && dataPtr+i+2 < dataLen; i++ {
				h := hash3(data[dataPtr+i:])
				chain := hashTable[h]
				copy(chain[1:], chain[0:chainLen-1])
				chain[0] = dataPtr + i
			}
		}
		dataPtr += advance

		// Update window for brute-force mode.
		if chainLen == 0 {
			start := dataPtr - windowSize
			if start < 0 {
				start = 0
			}
			window = data[start:dataPtr]
		}
	}

	// Process the final flag block.
	if flagCtr != 0 {
		flag <<= byte(7 - flagCtr)
		output[flagPtr] = flag
	} else {
		outPtr--
	}
	return output[:outPtr], nil
}

// Decompress decompresses LZSS compressed data.
func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
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
				return nil, errors.New("lzss: truncated match reference")
			}
			mark := binary.LittleEndian.Uint16(data[dataPtr:])
			offset := int(mark>>4 + 1)
			length := int(mark&0xF + minMatchLength)
			start := len(output) - offset
			if start < 0 {
				return nil, errors.New("lzss: invalid match offset")
			}
			// Copy length bytes, handling overlapping matches correctly.
			for i := 0; i < length; i++ {
				output = append(output, output[start+i])
			}
			dataPtr += 2
		} else {
			if dataPtr >= dataLen {
				return nil, errors.New("lzss: truncated literal")
			}
			output = append(output, data[dataPtr])
			dataPtr++
		}
		flagIdx++
	}
	return output, nil
}
