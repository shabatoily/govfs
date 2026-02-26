package badger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func openBenchDB(b *testing.B) (*BadgerVFS, func()) {
	dir, err := os.MkdirTemp("", "vfs-bench-badger-*")
	require.NoError(b, err)

	db, err := New(&Config{
		Path:   dir,
		Logger: logger,
	})
	require.NoError(b, err)

	return db, func() {
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
}

func Benchmark_BadgerVFS_Create(b *testing.B) {
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
			vfs, cleanup := openBenchDB(b)
			defer cleanup()

			data := make([]byte, bm.size)
			b.SetBytes(int64(bm.size))

			_, err := vfs.Mkdir("/bench/")
			require.NoError(b, err)

			b.StopTimer()
			paths := make([]string, b.N)
			for i := range b.N {
				paths[i] = fmt.Sprintf("/bench/%d", i)
			}
			b.StartTimer()

			for i := range b.N {
				_, err := vfs.Create(paths[i], bytes.NewReader(data))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func Benchmark_BadgerVFS_Read(b *testing.B) {
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
			vfs, cleanup := openBenchDB(b)
			defer cleanup()

			data := make([]byte, bm.size)
			meta, err := vfs.Create("/bench_read", bytes.NewReader(data))
			require.NoError(b, err)

			b.SetBytes(int64(bm.size))
			b.ResetTimer()

			for range b.N {
				f, err := vfs.Open(meta.ID)
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

func Benchmark_BadgerVFS_Seek(b *testing.B) {
	// 50MB 파일로 테스트 (청크 여러 개를 넘나드는 것을 시뮬레이션)
	size := 50 * 1024 * 1024
	data := make([]byte, size)

	vfs, cleanup := openBenchDB(b)
	defer cleanup()

	// 파일 1개 생성
	meta, err := vfs.Create("/bench_seek", bytes.NewReader(data))
	require.NoError(b, err)

	f, err := vfs.Open(meta.ID)
	require.NoError(b, err)
	defer f.Close()

	b.SetBytes(1024) // 1회 Seek마다 읽을 데이터 양 (가정)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// 임의의 위치로 점프 (0 ~ size-1024)
		// 벤치마크 안정성을 위해 단순한 패턴 사용: (i * 1024) % (size - 1024)
		offset := int64((i * 102400) % (size - 1024))

		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			b.Fatal(err)
		}

		// Seek 후 조금 읽기 (실제 로드 유발)
		buf := make([]byte, 1024)
		if _, err := f.Read(buf); err != nil {
			b.Fatal(err)
		}
	}
}
