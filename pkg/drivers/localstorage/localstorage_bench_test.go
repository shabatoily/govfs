package localstorage

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/shabatoily/govfs/pkg/log"
	"github.com/stretchr/testify/require"
)

func openBenchLS(b *testing.B) (*LocalStorage, func()) {
	dir, err := os.MkdirTemp("", "vfs-bench-local-*")
	require.NoError(b, err)
	ls, err := New(&Config{Path: dir, Logger: log.Default})
	require.NoError(b, err)

	return ls, func() {
		_ = ls.Close()
		_ = os.RemoveAll(dir)
	}
}

func Benchmark_LocalStorage_Create(b *testing.B) {
	benchmarks := []struct {
		name string
		size int
	}{
		{name: "1KB", size: 1024},
		{name: "4KB", size: 4096},
		{name: "1MB", size: 1024 * 1024},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			ls, cleanup := openBenchLS(b)
			defer cleanup()

			data := make([]byte, bm.size)
			b.SetBytes(int64(bm.size))

			_, err := ls.Mkdir("/bench/")
			require.NoError(b, err)

			b.StopTimer()
			paths := make([]string, b.N)
			for i := range b.N {
				paths[i] = fmt.Sprintf("/bench/%d", i)
			}
			b.StartTimer()

			for i := range b.N {
				_, err := ls.Create(paths[i], bytes.NewReader(data))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func Benchmark_LocalStorage_Read(b *testing.B) {
	benchmarks := []struct {
		name string
		size int
	}{
		{name: "1KB", size: 1024},
		{name: "4KB", size: 4096},
		{name: "1MB", size: 1024 * 1024},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			ls, cleanup := openBenchLS(b)
			defer cleanup()

			data := make([]byte, bm.size)
			meta, err := ls.Create("/bench_read", bytes.NewReader(data))
			require.NoError(b, err)

			b.SetBytes(int64(bm.size))
			b.ResetTimer()

			for range b.N {
				f, err := ls.Open(meta.ID)
				if err != nil {
					b.Fatal(err)
				}
				_, err = io.Copy(io.Discard, f)
				if err != nil {
					b.Fatal(err)
				}
				_ = f.Close()
			}
		})
	}
}

func Benchmark_LocalStorage_Seek(b *testing.B) {
	// 50MB 파일로 테스트
	size := 50 * 1024 * 1024
	data := make([]byte, size)

	ls, cleanup := openBenchLS(b)
	defer cleanup()

	// 파일 1개 생성
	meta, err := ls.Create("/bench_seek", bytes.NewReader(data))
	require.NoError(b, err)

	f, err := ls.Open(meta.ID)
	require.NoError(b, err)
	defer f.Close()

	b.SetBytes(1024)
	b.ResetTimer()

	for i := range b.N {
		offset := int64((i * 102400) % (size - 1024))

		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			b.Fatal(err)
		}

		buf := make([]byte, 1024)
		if _, err := f.Read(buf); err != nil {
			b.Fatal(err)
		}
	}
}
