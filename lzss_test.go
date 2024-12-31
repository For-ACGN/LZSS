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

	now := time.Now()
	data := Compress(raw)
	fmt.Printf("compress time: %d ms\n", time.Since(now).Milliseconds())

	ratio := (1 - float32(len(data))/float32(len(raw))) * 100
	fmt.Printf("%d/%d %.2f%%\n", len(data), len(raw), ratio)

	now = time.Now()
	data = Decompress(data, len(raw))
	fmt.Printf("decompress time: %d ms\n", time.Since(now).Milliseconds())
	require.Equal(t, raw, data)

	spew.Dump(raw[:100])
	spew.Dump(data[:100])
}
