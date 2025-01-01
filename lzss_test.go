package lzss

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
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
		data = Decompress(data, len(raw))
		fmt.Printf("decompress time: %d ms\n", time.Since(now).Milliseconds())
		require.Equal(t, raw, data)

		spew.Dump(raw[:100])
		spew.Dump(data[:100])
	})

	t.Run("invalid window size", func(t *testing.T) {
		data, err := Compress(raw, maxWindowSize+1)
		require.EqualError(t, err, "invalid window size")
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

			data = Decompress(data, len(raw))
			require.Equal(t, raw, data)
		}
	})
}
