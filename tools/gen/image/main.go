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
	"strconv"
)

func main() {
	var (
		width  int
		height int
		count  int
		outDir string
	)

	flag.IntVar(&width, "width", 640, "Image width")
	flag.IntVar(&height, "height", 480, "Image height")
	flag.IntVar(&count, "count", 1, "Number of images to generate")
	flag.StringVar(&outDir, "out", ".", "Output directory")
	flag.Parse()

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	for i := 0; i < count; i++ {
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

	// Fill background with a unique color based on index
	bgColor := color.RGBA{
		R: uint8((index * 50) % 255),
		G: uint8((index * 80) % 255),
		B: uint8((index * 110) % 255),
		A: 255,
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
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 4; dx++ {
					img.Set(x+dx, y+dy, color.White)
				}
			}
		}
	}

	// Draw index number (simulated by a colored block at top-left)
	indexColor := color.RGBA{uint8(index % 255), 255, 0, 255}
	draw.Draw(img, image.Rect(10, 10, 60, 60), &image.Uniform{indexColor}, image.Point{}, draw.Src)

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

func parseColor(s string) color.RGBA {
	// Simple helper if we wanted custom colors, currently unused but good for extension
	// expecting hex string like "#RRGGBB"
	if len(s) == 7 && s[0] == '#' {
		r, _ := strconv.ParseUint(s[1:3], 16, 8)
		g, _ := strconv.ParseUint(s[3:5], 16, 8)
		b, _ := strconv.ParseUint(s[5:7], 16, 8)
		return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
	}
	return color.RGBA{0, 0, 0, 255} // Default black
}
