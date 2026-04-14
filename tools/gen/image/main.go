package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

const (
	defaultDirMode = 0755

	mod = 255

	x0 = 10
	y0 = 10
	x1 = 60
	y1 = 60
)

func main() {
	width := 640
	height := 480
	count := 1
	outDir := "."

	flag.IntVar(&width, "width", width, "Image width")
	flag.IntVar(&height, "height", height, "Image height")
	flag.IntVar(&count, "count", count, "Number of images to generate")
	flag.StringVar(&outDir, "out", outDir, "Output directory")
	flag.Parse()

	if err := os.MkdirAll(outDir, defaultDirMode); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	for i := range count {
		filename := filepath.Join(outDir, fmt.Sprintf("image_%d_%dx%d.png", i, width, height))
		if err := generateImage(filename, width, height, i); err != nil {
			fmt.Printf("Error generating image %s: %v\n", filename, err)
		} else {
			fmt.Printf("Generated %s\n", filename)
		}
	}
}

func generateImage(filename string, w, h, index int) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	if index < 0 {
		return fmt.Errorf("invalid index: %d", index)
	}

	// Fill background with a unique color based on index
	bgColor := color.RGBA{
		R: uint8((index * 50) % mod),
		G: uint8((index * 80) % mod),
		B: uint8((index * 110) % mod),
		A: uint8(mod),
	}

	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	// Draw a diagonal line
	for i := 0; i < w && i < h; i++ {
		img.Set(i, i, color.White)
		img.Set(w-1-i, i, color.White)
	}

	// Add text-like pattern (simple dots) to indicate "content"
	gridSize := 40
	for y := 20; y < h-20; y += gridSize {
		for x := 20; x < w-20; x += gridSize {
			// Draw filled square
			for dy := range 4 {
				for dx := range 4 {
					img.Set(x+dx, y+dy, color.White)
				}
			}
		}
	}

	// Draw index number (simulated by a colored block at top-left)
	indexColor := color.RGBA{uint8(index % mod), uint8(mod), 0, uint8(mod)}
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{indexColor}, image.Point{}, draw.Src)

	// Draw size text (simulated)
	// Real text drawing requires external font parsing which is heavy for a "dummy" generator,
	// so we keep it simple with geometric shapes.

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// PNG for lossless, widely supported dummy image
	return png.Encode(f, img)
}
