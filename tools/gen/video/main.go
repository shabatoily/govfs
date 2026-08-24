package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const dirFileMode = 0o755

func main() {
	width := 640
	height := 480
	duration := 5
	count := 1
	outDir := "."
	fps := 24

	flag.IntVar(&width, "width", width, "Video width")
	flag.IntVar(&height, "height", height, "Video height")
	flag.IntVar(&duration, "duration", duration, "Video duration in seconds")
	flag.IntVar(&count, "count", count, "Number of videos to generate")
	flag.StringVar(&outDir, "out", outDir, "Output directory")
	flag.IntVar(&fps, "fps", fps, "Frames per second")
	flag.Parse()

	if err := os.MkdirAll(outDir, dirFileMode); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Check if ffmpeg is available
	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		fmt.Println("Error: 'ffmpeg' is required but not found in PATH.")
		fmt.Println("Please install ffmpeg to generate dummy videos.")
		os.Exit(1)
	}

	ctx := context.Background()

	for i := range count {
		filename := filepath.Join(outDir, fmt.Sprintf("video_%d_%dx%d_%ds.mp4", i, width, height, duration))
		if err := generateVideo(ctx, filename, width, height, duration, fps); err != nil {
			fmt.Printf("Error generating video %s: %v\n", filename, err)
		} else {
			fmt.Printf("Generated %s\n", filename)
		}
	}
}

func generateVideo(ctx context.Context, filename string, w, h, duration, fps int) error {
	// Use ffmpeg to generate a test pattern video
	// -f lavfi -i testsrc:size=WxH:rate=FPS:duration=SEC
	// -c:v libx264 (H.264 is widely supported and lightweight enough for dummy)
	// -pix_fmt yuv420p (for compatibility)
	// Add text overlay with frame number/timestamp using drawtext (optional, complicates command line if fonts missing)
	// We will stick to basic testsrc which includes a counter.

	size := fmt.Sprintf("%dx%d", w, h)
	rate := fmt.Sprintf("%d", fps)
	dur := fmt.Sprintf("%d", duration)

	// Unique color for each video index to distinguish them visually if needed
	// changing background color of testsrc is not straightforward without complex filters,
	// so we'll just stick to standard testsrc.

	//nolint:gosec // 파일 경로는 내부 로직(filepath.Join)에 의해 생성되며, 파일명은 고정된 포맷을 따름
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",          // Overwrite output
		"-f", "lavfi", // Input format: lavfi (Libavfilter input virtual device)
		"-i", fmt.Sprintf("testsrc=size=%s:rate=%s:duration=%s", size, rate, dur), // Input source
		"-c:v", "libx264", // Video codec
		"-pix_fmt", "yuv420p", // Pixel format for compatibility
		"-tune", "fastdecode", // Tune for decoding speed (lightweight)
		filename,
	)

	// Capture output for debugging if needed, but for now just run it.
	// fmt.Println(cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
	}

	return nil
}
