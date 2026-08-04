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

	t.Run("common", func(t *testing.T) {
		now := time.Now()
		data, err := Compress(raw, 0)
		require.NoError(t, err)
		fmt.Printf("compress time: %d ms\n", time.Since(now).Milliseconds())

		ratio := (1 - float32(len(data))/float32(len(raw))) * 100
		fmt.Printf("%d/%d %.2f%%\n", len(data), len(raw), ratio)

		now = time.Now()
		decompressed, err := Decompress(data)
		require.NoError(t, err)
		fmt.Printf("decompress time: %d ms\n", time.Since(now).Milliseconds())
		require.Equal(t, raw, decompressed)
	})

	t.Run("invalid window size", func(t *testing.T) {
		data, err := Compress(raw, maximumWindowSize+1)
		require.EqualError(t, err, "lzss: invalid window size")
		require.Nil(t, data)
	})

	t.Run("invalid chain length", func(t *testing.T) {
		data, err := CompressWith(raw, 0, -1)
		require.EqualError(t, err, "lzss: invalid chain length")
		require.Nil(t, data)
	})

	t.Run("various window size", func(t *testing.T) {
		for _, windowSize := range []int{
			32, 64, 128, 256, 512,
			1024, 1536, 2048, 4096,
		} {
			fmt.Println("window size:", windowSize)

			now := time.Now()
			data, err := Compress(raw, windowSize)
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
}

func TestCompressWith(t *testing.T) {
	raw, err := os.ReadFile("testdata/gofmt.dat")
	require.NoError(t, err)

	for _, cl := range []int{0, 1, 3, 6, 8} {
		t.Run(fmt.Sprintf("chainLen=%d", cl), func(t *testing.T) {
			now := time.Now()
			data, err := CompressWith(raw, 4096, cl)
			require.NoError(t, err)
			elapsed := time.Since(now)

			ratio := (1 - float32(len(data))/float32(len(raw))) * 100
			fmt.Printf("chainLen=%d: %d ms, %d/%d, ratio: %.2f%%\n",
				cl, elapsed.Milliseconds(), len(data), len(raw), ratio)

			decompressed, err := Decompress(data)
			require.NoError(t, err)
			require.Equal(t, raw, decompressed)
		})
	}
}

func TestCompress_Fuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for _, cl := range []int{0, 1, 3, 6} {
		t.Run(fmt.Sprintf("chainLen=%d", cl), func(t *testing.T) {
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
				data, err := CompressWith(raw, 1024, cl)
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
	raw = append(raw, bytes.Repeat([]byte{0x00}, 30*1024*1024)...)

	for _, cl := range []int{0, 1, 3, 6, 8} {
		b.Run(fmt.Sprintf("chainLen=%d", cl), func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := CompressWith(raw, 4096, cl)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecompress(b *testing.B) {
	raw, err := os.ReadFile("testdata/gofmt.dat")
	if err != nil {
		b.Fatal(err)
	}
	compressed, err := Compress(raw, 0)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(compressed)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Decompress(compressed)
		if err != nil {
			b.Fatal(err)
		}
	}
}
