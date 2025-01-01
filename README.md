# LZSS
A simple LZSS implementation using marker bit grouping.

## Usage
```go
package main

import (
    "bytes"
    "fmt"
    "os"

    "github.com/For-ACGN/LZSS"
)

func main() {
    raw, err := os.ReadFile("test.dat")
    checkError(err)

    output, err := lzss.Compress(raw, 1024)
    checkError(err)

    output = lzss.Decompress(output, len(raw))
    fmt.Println(bytes.Equal(raw, output))
}

func checkError(err error) {
    if err != nil {
        fmt.Println(err)
        os.Exit(1)
    }
}
```
