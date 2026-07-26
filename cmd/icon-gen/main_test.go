package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPNGIsCenteredAndSizedForTrayUse(t *testing.T) {
	t.Parallel()

	source := readSource(t)
	rendered, err := renderPNG(source, 256)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodePNG(t, rendered)
	visible := visibleBounds(decoded)
	if visible.Empty() {
		t.Fatal("rendered icon is fully transparent")
	}

	largestDimension := max(visible.Dx(), visible.Dy())
	coverage := float64(largestDimension) / 256
	if coverage < 0.80 || coverage > 0.84 {
		t.Fatalf("visible bounds = %v (%.2f%% of canvas), want 80%% to 84%%", visible, coverage*100)
	}
	centerX := float64(visible.Min.X+visible.Max.X) / 2
	centerY := float64(visible.Min.Y+visible.Max.Y) / 2
	if math.Abs(centerX-128) > 2.56 || math.Abs(centerY-128) > 2.56 {
		t.Fatalf("visible center = (%.2f, %.2f), want within 1%% of (128, 128)", centerX, centerY)
	}
}

func TestTemplateIconsAreMonochromeWithMatchingNegativeSpace(t *testing.T) {
	t.Parallel()

	source := readSource(t)
	blackPNG, err := renderTemplatePNG(source, macOSTraySize, "#000000")
	if err != nil {
		t.Fatal(err)
	}
	whitePNG, err := renderTemplatePNG(source, macOSTraySize, "#ffffff")
	if err != nil {
		t.Fatal(err)
	}
	black := decodePNG(t, blackPNG)
	white := decodePNG(t, whitePNG)

	for y := 0; y < macOSTraySize; y++ {
		for x := 0; x < macOSTraySize; x++ {
			blackPixel := color.NRGBAModel.Convert(black.At(x, y)).(color.NRGBA)
			whitePixel := color.NRGBAModel.Convert(white.At(x, y)).(color.NRGBA)
			if blackPixel.A != whitePixel.A {
				t.Fatalf("alpha differs at (%d, %d): black=%d white=%d", x, y, blackPixel.A, whitePixel.A)
			}
			if blackPixel.A == 0 {
				continue
			}
			if blackPixel.R != 0 || blackPixel.G != 0 || blackPixel.B != 0 {
				t.Fatalf("black template pixel at (%d, %d) = %#v", x, y, blackPixel)
			}
			if whitePixel.R != 0xff || whitePixel.G != 0xff || whitePixel.B != 0xff {
				t.Fatalf("white template pixel at (%d, %d) = %#v", x, y, whitePixel)
			}
		}
	}

	assertTemplateAlpha(t, black, templatePixel(359.18, 454), false, "left eye")
	assertTemplateAlpha(t, black, templatePixel(665.73, 453.99), false, "right eye")
	assertTemplateAlpha(t, black, templatePixel(230, 260), false, "left inner ear")
	assertTemplateAlpha(t, black, templatePixel(794, 260), false, "right inner ear")
	assertTemplateAlpha(t, black, templatePixel(512, 400), true, "face")
	assertTemplateAlpha(t, black, templatePixel(512, 770), true, "nose")
}

func TestStatusDotUsesSpecifiedGeometryAndColors(t *testing.T) {
	t.Parallel()

	for _, size := range icoSizes {
		basePNG, err := renderPNG(readSource(t), size)
		if err != nil {
			t.Fatalf("render %dpx base icon: %v", size, err)
		}
		base := toNRGBA(decodePNG(t, basePNG))
		center := float64(size) * statusDotCenter
		radius := float64(size) * statusDotDiameter / 2
		centerPixel := int(math.Floor(center))

		for _, status := range windowsStatuses {
			dottedPNG, renderErr := addStatusDot(basePNG, size, status.color)
			if renderErr != nil {
				t.Fatalf("render %s %dpx status icon: %v", status.name, size, renderErr)
			}
			dotted := toNRGBA(decodePNG(t, dottedPNG))
			if got := dotted.NRGBAAt(centerPixel, centerPixel); got != status.color {
				t.Fatalf("%s %dpx dot center = %#v, want %#v", status.name, size, got, status.color)
			}

			changed := image.Rectangle{}
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					got := dotted.NRGBAAt(x, y)
					want := base.NRGBAAt(x, y)
					distance := math.Hypot(float64(x)+0.5-center, float64(y)+0.5-center)
					if distance > radius+1 && got != want {
						t.Fatalf(
							"%s %dpx pixel outside dot changed at (%d, %d): got=%#v want=%#v",
							status.name,
							size,
							x,
							y,
							got,
							want,
						)
					}
					if got != want {
						point := image.Rect(x, y, x+1, y+1)
						if changed.Empty() {
							changed = point
						} else {
							changed = changed.Union(point)
						}
					}
				}
			}
			if changed.Empty() {
				t.Fatalf("%s %dpx status dot did not change any pixels", status.name, size)
			}
			changedDiameter := float64(max(changed.Dx(), changed.Dy()))
			expectedDiameter := min(
				float64(size)*statusDotDiameter,
				float64(size)-(center-radius),
			)
			if math.Abs(changedDiameter-expectedDiameter) > 2 {
				t.Fatalf(
					"%s %dpx changed bounds = %v (%.2fpx), want %.2fpx within antialiasing tolerance",
					status.name,
					size,
					changed,
					changedDiameter,
					expectedDiameter,
				)
			}
		}
	}
}

func TestPlatformContainersHaveExpectedSignaturesAndSizes(t *testing.T) {
	t.Parallel()

	images := make(map[int][]byte)
	for _, size := range renderSizes {
		images[size] = []byte{byte(size)}
	}

	ico := encodeICO(images)
	if !bytes.Equal(ico[:4], []byte{0, 0, 1, 0}) {
		t.Fatalf("ICO signature = %v", ico[:4])
	}
	assertICOSizes(t, ico, icoSizes)
	icns := encodeICNS(images)
	if string(icns[:4]) != "icns" {
		t.Fatalf("ICNS signature = %q", icns[:4])
	}
}

func TestEncodeWindowsResourceIsDeterministicAMD64COFF(t *testing.T) {
	t.Parallel()

	source := readSource(t)
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

func readSource(t *testing.T) []byte {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "assets", "kitsune.svg"))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func decodePNG(t *testing.T, encoded []byte) image.Image {
	t.Helper()
	decoded, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func toNRGBA(source image.Image) *image.NRGBA {
	target := image.NewNRGBA(source.Bounds())
	draw.Draw(target, target.Bounds(), source, source.Bounds().Min, draw.Src)
	return target
}

func visibleBounds(source image.Image) image.Rectangle {
	visible := image.Rectangle{}
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			if alpha == 0 {
				continue
			}
			point := image.Rect(x, y, x+1, y+1)
			if visible.Empty() {
				visible = point
			} else {
				visible = visible.Union(point)
			}
		}
	}
	return visible
}

func templatePixel(sourceX, sourceY float64) image.Point {
	const (
		scale      = 1.190123877
		translateX = -97.242264
		translateY = -26.162116
	)
	x := (sourceX*scale + translateX) * macOSTraySize / 1024
	y := (sourceY*scale + translateY) * macOSTraySize / 1024
	return image.Pt(int(math.Floor(x)), int(math.Floor(y)))
}

func assertTemplateAlpha(t *testing.T, source image.Image, point image.Point, visible bool, name string) {
	t.Helper()
	_, _, _, alpha := source.At(point.X, point.Y).RGBA()
	if visible && alpha == 0 {
		t.Errorf("%s at %v is transparent", name, point)
	}
	if !visible && alpha != 0 {
		t.Errorf("%s at %v has alpha %d, want transparent", name, point, alpha)
	}
}

func assertICOSizes(t *testing.T, ico []byte, want []int) {
	t.Helper()
	if len(ico) < 6 {
		t.Fatal("ICO header is truncated")
	}
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	if count != len(want) || len(ico) < 6+count*16 {
		t.Fatalf("ICO image count = %d, want %d", count, len(want))
	}

	got := make(map[int]bool, count)
	for index := 0; index < count; index++ {
		width := int(ico[6+index*16])
		height := int(ico[6+index*16+1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		if width != height {
			t.Fatalf("ICO image %d dimensions = %dx%d", index, width, height)
		}
		got[width] = true
	}
	for _, size := range want {
		if !got[size] {
			t.Errorf("ICO does not contain %dx%d image", size, size)
		}
	}
}
