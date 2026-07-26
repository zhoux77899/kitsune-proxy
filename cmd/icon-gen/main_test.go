package main

import (
	"bytes"
	"debug/pe"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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

func TestEncodeWindowsResourceIsDeterministicAMD64COFF(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(filepath.Join("..", "..", "assets", "kitsune.svg"))
	if err != nil {
		t.Fatal(err)
	}
	images := make(map[int][]byte)
	for _, size := range icoSizes {
		rendered, renderErr := renderPNG(source, size)
		if renderErr != nil {
			t.Fatalf("render %dpx icon: %v", size, renderErr)
		}
		images[size] = rendered
	}

	ico := encodeICO(images)
	first, err := encodeWindowsResource(ico)
	if err != nil {
		t.Fatal(err)
	}
	second, err := encodeWindowsResource(ico)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Windows resource output is not deterministic")
	}

	manifestMarkers := [][]byte{
		[]byte(`<dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true</dpiAware>`),
		[]byte(`<dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">permonitorv2,system</dpiAwareness>`),
	}
	for _, marker := range manifestMarkers {
		if !bytes.Contains(first, marker) {
			t.Fatalf("Windows resource does not contain DPI manifest marker %q", marker)
		}
	}

	object, err := pe.NewFile(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("open Windows resource as COFF: %v", err)
	}
	defer object.Close()

	if object.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		t.Fatalf("COFF machine = %#x, want amd64", object.Machine)
	}

	for _, section := range object.Sections {
		if !strings.HasPrefix(section.Name, ".rsrc") || section.Size == 0 {
			continue
		}
		if content, readErr := section.Data(); readErr != nil {
			t.Fatalf("read %s section: %v", section.Name, readErr)
		} else if len(content) > 0 {
			return
		}
	}
	t.Fatal("Windows resource has no non-empty .rsrc section")
}
