package lzss

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCompress(t *testing.T) {
	raw, err := os.ReadFile("testdata/gofmt.dat")
	require.NoError(t, err)

	t.Run("single hash candidate", func(t *testing.T) {
		testCompress(t, raw, 0, MinimumChainLen)
	})

	t.Run("n hash candidate", func(t *testing.T) {
		testCompress(t, raw, 0, DefaultChainLen)
	})

	t.Run("brute force", func(t *testing.T) {
		testCompress(t, raw, 0, MaximumChainLen)
	})

	t.Run("default window and chain", func(t *testing.T) {
		testCompress(t, raw, 0, 0)
	})

	t.Run("various window size", func(t *testing.T) {
		for _, windowSize := range []int{
			128, 256, 512, 1024,
			1536, 2048, 3072, 4096,
		} {
			fmt.Println("window size:", windowSize)

			now := time.Now()
			data, err := Compress(raw, windowSize, 0)
			require.NoError(t, err)
			fmt.Printf("compress time: %d ms\n", time.Since(now).Milliseconds())

			ratio := (1 - float32(len(data))/float32(len(raw))) * 100
			fmt.Printf("%d/%d, ratio: %.2f%%\n", len(data), len(raw), ratio)
			fmt.Println()

			decompressed, err := Decompress(data)
			require.NoError(t, err)
			require.Equal(t, raw, decompressed)
		}
	})

	t.Run("invalid windows size", func(t *testing.T) {
		data, err := Compress(raw, MaximumWindowSize+1, 0)
		require.EqualError(t, err, "invalid window size")
		require.Nil(t, data)
	})

	t.Run("invalid chain length", func(t *testing.T) {
		data, err := Compress(raw, DefaultWindowSize, -1)
		require.EqualError(t, err, "invalid chain length")
		require.Nil(t, data)
	})
}

func testCompress(t *testing.T, raw []byte, windowSize, chainLen int) {
	now := time.Now()
	compressed, err := Compress(raw, windowSize, chainLen)
	require.NoError(t, err)
	fmt.Printf("compress time: %d ms\n", time.Since(now).Milliseconds())

	ratio := (1 - float32(len(compressed))/float32(len(raw))) * 100
	fmt.Printf("%d/%d %.2f%%\n", len(compressed), len(raw), ratio)

	now = time.Now()
	decompressed, err := Decompress(compressed)
	require.NoError(t, err)
	fmt.Printf("decompress time: %d ms\n", time.Since(now).Milliseconds())
	require.Equal(t, raw, decompressed)
}

func TestDecompress(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result, err := Decompress(nil)
		require.NoError(t, err)
		require.Nil(t, result)

		result, err = Decompress([]byte{})
		require.NoError(t, err)
		require.Nil(t, result)
	})

	t.Run("all literals exact flag boundary", func(t *testing.T) {
		// flag=0x00: all 8 elements are literals.
		data := []byte{0x00, 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'}
		result, err := Decompress(data)
		require.NoError(t, err)
		require.Equal(t, []byte("ABCDEFGH"), result)
	})

	t.Run("literals with partial flag", func(t *testing.T) {
		// Only 3 elements in the group; remaining flag bits are 0 (literals).
		// Format: flag_byte + 3 literals.
		// The loop exits when dataPtr reaches dataLen before processing remaining bits.
		data := []byte{0x00, 'X', 'Y', 'Z'}
		result, err := Decompress(data)
		require.NoError(t, err)
		require.Equal(t, []byte("XYZ"), result)
	})

	t.Run("simple match", func(t *testing.T) {
		// Compress "AAAA": literal 'A' + match(offset=1, length=3).
		// flag=0x40 (bit7=0 literal, bit6=1 match, rest 0 literal).
		// mark = (offset-1)<<4 | (length-3) = 0<<4 | 0 = 0x0000.
		// Plus 6 filler literals to complete the flag group (total 10 bytes).
		flag := byte(0x40) // 0100_0000
		matchMark := []byte{0x00, 0x00}
		data := append([]byte{flag, 'A'}, matchMark...)
		data = append(data, 'X', 'X', 'X', 'X', 'X', 'X') // 6 filler literals
		result, err := Decompress(data)
		require.NoError(t, err)
		require.Equal(t, []byte("AAAAXXXXXX"), result)
	})

	t.Run("overlapping match", func(t *testing.T) {
		// Decompress "AB" + match(offset=1, length=4) → "ABBBBB".
		// flag=0x20 (bit7=0, bit6=0, bit5=1, rest 0).
		// mark = ((1-1)<<4) | (4-3) = 0x0001.
		// 5 filler literals to complete the group (total 10 bytes).
		flag := byte(0x20) // 0010_0000
		matchMark := []byte{0x01, 0x00}
		data := append([]byte{flag, 'A', 'B'}, matchMark...)
		data = append(data, 'X', 'X', 'X', 'X', 'X') // 5 filler literals
		result, err := Decompress(data)
		require.NoError(t, err)
		require.Equal(t, []byte("ABBBBBXXXXX"), result)
	})

	t.Run("truncated match reference", func(t *testing.T) {
		// flag=0x80 means bit7=1 (match), but only 1 data byte after flag.
		data := []byte{0x80, 0x00}
		result, err := Decompress(data)
		require.EqualError(t, err, "truncated match reference")
		require.Nil(t, result)
	})

	t.Run("truncated literal", func(t *testing.T) {
		// Single flag byte with no following data — first element is literal.
		data := []byte{0x00}
		result, err := Decompress(data)
		require.EqualError(t, err, "truncated literal")
		require.Nil(t, result)
	})

	t.Run("truncated literal after flag read", func(t *testing.T) {
		// Data ends exactly after reading a new flag byte, before processing any element.
		// flag=0x7F (bit7=0 → literal, all others = 1 → matches).
		// After reading the 1-byte flag, data is exhausted and element 0 (literal) fails.
		data := []byte{0x7F}
		result, err := Decompress(data)
		require.EqualError(t, err, "truncated literal")
		require.Nil(t, result)
	})

	t.Run("invalid match offset", func(t *testing.T) {
		// First element literal 'A' (output len=1), second element match with offset=2.
		// offset=2 > output len → "invalid match offset".
		// flag=0x40 (0100_0000): bit7=0 literal, bit6=1 match.
		// mark = ((2-1)<<4) | 0 = 0x10.
		data := []byte{0x40, 'A', 0x10, 0x00}
		result, err := Decompress(data)
		require.EqualError(t, err, "invalid match offset")
		require.Nil(t, result)
	})
}

func TestCompress_Fuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for _, cl := range []int{
		MinimumChainLen,
		3, 6, 9,
		MaximumChainLen,
	} {
		t.Run(fmt.Sprintf("chain len=%d", cl), func(t *testing.T) {
			for i := 0; i < 250; i++ {
				raw := make([]byte, 0, 32*1024)
				for j := 0; j < 1000; j++ {
					switch rng.Intn(2) {
					case 0:
						for k := 0; k < 64; k++ {
							raw = append(raw, byte(rng.Intn(4)))
						}
					case 1:
						for k := 0; k < 32; k++ {
							raw = append(raw, byte(rng.Intn(6)))
						}
					}
				}
				data, err := Compress(raw, DefaultWindowSize, cl)
				require.NoError(t, err)
				decompressed, err := Decompress(data)
				require.NoError(t, err)
				require.Equal(t, raw, decompressed)
			}
		})
	}
}

func BenchmarkCompress(b *testing.B) {
	raw, err := os.ReadFile("testdata/gofmt.dat")
	if err != nil {
		b.Fatal(err)
	}

	b.Run("common executable", func(b *testing.B) {
		for _, cl := range []int{
			MinimumChainLen,
			3, 6, 9,
			MaximumChainLen,
		} {
			b.Run(fmt.Sprintf("chain len=%d", cl), func(b *testing.B) {
				b.SetBytes(int64(len(raw)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_, err = Compress(raw, MaximumWindowSize, cl)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	})

	b.Run("with duplicate data", func(b *testing.B) {
		// append duplicate data
		raw = bytes.Clone(raw)
		raw = append(raw, bytes.Repeat([]byte{0x00}, 32*1024*1024)...)

		for _, cl := range []int{
			MinimumChainLen,
			3, 6, 9,
			MaximumChainLen,
		} {
			b.Run(fmt.Sprintf("chain len=%d", cl), func(b *testing.B) {
				b.SetBytes(int64(len(raw)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_, err = Compress(raw, MaximumWindowSize, cl)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	})
}

func BenchmarkDecompress(b *testing.B) {
	raw, err := os.ReadFile("testdata/gofmt.dat")
	if err != nil {
		b.Fatal(err)
	}
	compressed, err := Compress(raw, MaximumWindowSize, MaximumChainLen)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = Decompress(compressed)
		if err != nil {
			b.Fatal(err)
		}
	}
}
