package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderPNGContainsVisibleSourceArtwork(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(filepath.Join("..", "..", "assets", "kitsune.svg"))
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderPNG(source, 256)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(rendered))
	if err != nil {
		t.Fatal(err)
	}

	visible := 0
	for y := decoded.Bounds().Min.Y; y < decoded.Bounds().Max.Y; y++ {
		for x := decoded.Bounds().Min.X; x < decoded.Bounds().Max.X; x++ {
			_, _, _, alpha := decoded.At(x, y).RGBA()
			if alpha != 0 {
				visible++
			}
		}
	}
	if visible < 10_000 {
		t.Fatalf("rendered icon has only %d visible pixels", visible)
	}
}

func TestPlatformContainersHaveExpectedSignatures(t *testing.T) {
	t.Parallel()

	images := make(map[int][]byte)
	for _, size := range []int{16, 20, 24, 32, 48, 64, 128, 256, 512, 1024} {
		images[size] = []byte{byte(size)}
	}

	ico := encodeICO(images)
	if !bytes.Equal(ico[:4], []byte{0, 0, 1, 0}) {
		t.Fatalf("ICO signature = %v", ico[:4])
	}
	icns := encodeICNS(images)
	if string(icns[:4]) != "icns" {
		t.Fatalf("ICNS signature = %q", icns[:4])
	}
}
