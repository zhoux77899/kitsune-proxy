// Command icon-gen reproducibly renders platform icons from assets/kitsune.svg.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"github.com/tc-hib/winres"
)

const (
	macOSTraySize     = 32
	statusDotCenter   = 0.84
	statusDotDiameter = 0.34
)

var (
	icoSizes        = []int{16, 20, 24, 32, 48, 64, 128, 256}
	renderSizes     = []int{16, 20, 24, 32, 48, 64, 128, 256, 512, 1024}
	windowsStatuses = []struct {
		name  string
		color color.NRGBA
	}{
		{name: "healthy", color: color.NRGBA{R: 0x10, G: 0x7c, B: 0x10, A: 0xff}},
		{name: "degraded", color: color.NRGBA{R: 0xff, G: 0xb9, B: 0x00, A: 0xff}},
		{name: "error", color: color.NRGBA{R: 0xd1, G: 0x34, B: 0x38, A: 0xff}},
		{name: "stopped", color: color.NRGBA{R: 0x8a, G: 0x88, B: 0x86, A: 0xff}},
	}
)

func main() {
	source, err := os.ReadFile(filepath.Join("assets", "kitsune.svg"))
	if err != nil {
		fatal(err)
	}

	outputDir := filepath.Join("assets", "generated")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fatal(err)
	}

	images := make(map[int][]byte, len(renderSizes))
	for _, size := range renderSizes {
		rendered, renderErr := renderPNG(source, size)
		if renderErr != nil {
			fatal(fmt.Errorf("render %dpx icon: %w", size, renderErr))
		}
		images[size] = rendered
	}

	ico := encodeICO(images)
	icns := encodeICNS(images)
	windowsResource, err := encodeWindowsResource(ico)
	if err != nil {
		fatal(fmt.Errorf("encode Windows application icon resource: %w", err))
	}

	writeGenerated(outputDir, "kitsune.png", images[256])
	writeGenerated(outputDir, "kitsune.ico", ico)
	writeGenerated(outputDir, "kitsune.icns", icns)

	for _, status := range windowsStatuses {
		statusImages := make(map[int][]byte, len(icoSizes))
		for _, size := range icoSizes {
			rendered, renderErr := addStatusDot(images[size], size, status.color)
			if renderErr != nil {
				fatal(fmt.Errorf("render Windows %s %dpx icon: %w", status.name, size, renderErr))
			}
			statusImages[size] = rendered
		}
		writeGenerated(
			outputDir,
			fmt.Sprintf("kitsune-tray-windows-%s.ico", status.name),
			encodeICO(statusImages),
		)
	}

	macOSBlack, err := renderTemplatePNG(source, macOSTraySize, "#000000")
	if err != nil {
		fatal(fmt.Errorf("render macOS black tray icon: %w", err))
	}
	macOSWhite, err := renderTemplatePNG(source, macOSTraySize, "#ffffff")
	if err != nil {
		fatal(fmt.Errorf("render macOS white tray icon: %w", err))
	}
	writeGenerated(outputDir, "kitsune-tray-macos-black.png", macOSBlack)
	writeGenerated(outputDir, "kitsune-tray-macos-white.png", macOSWhite)

	if err := os.WriteFile(
		filepath.Join("cmd", "kitsune-proxy", "rsrc_windows_amd64.syso"),
		windowsResource,
		0o644,
	); err != nil {
		fatal(err)
	}
}

func writeGenerated(outputDir, name string, content []byte) {
	if err := os.WriteFile(filepath.Join(outputDir, name), content, 0o644); err != nil {
		fatal(err)
	}
}

func renderPNG(source []byte, size int) ([]byte, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(source))
	if err != nil {
		return nil, err
	}
	// The transform order maps exported viewBox coordinates into target pixels.
	scaleX := float64(size) / icon.ViewBox.W
	scaleY := float64(size) / icon.ViewBox.H
	icon.Transform = rasterx.Identity.
		Scale(scaleX, scaleY).
		Translate(-icon.ViewBox.X, -icon.ViewBox.Y)

	target := image.NewRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, target, target.Bounds())
	dasher := rasterx.NewDasher(size, size, scanner)
	icon.Draw(dasher, 1)

	return encodePNG(target)
}

func renderTemplatePNG(source []byte, size int, templateColor string) ([]byte, error) {
	solidSource, cutoutSource, err := buildTemplateLayers(source)
	if err != nil {
		return nil, err
	}
	solidPNG, err := renderPNG(solidSource, size)
	if err != nil {
		return nil, fmt.Errorf("render solid template layer: %w", err)
	}
	cutoutPNG, err := renderPNG(cutoutSource, size)
	if err != nil {
		return nil, fmt.Errorf("render cutout template layer: %w", err)
	}
	solid, err := png.Decode(bytes.NewReader(solidPNG))
	if err != nil {
		return nil, fmt.Errorf("decode solid template layer: %w", err)
	}
	cutout, err := png.Decode(bytes.NewReader(cutoutPNG))
	if err != nil {
		return nil, fmt.Errorf("decode cutout template layer: %w", err)
	}

	var foreground color.NRGBA
	switch templateColor {
	case "#000000":
		foreground = color.NRGBA{A: 0xff}
	case "#ffffff":
		foreground = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	default:
		return nil, fmt.Errorf("unsupported template color %q", templateColor)
	}

	target := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			_, _, _, solidAlpha16 := solid.At(x, y).RGBA()
			_, _, _, cutoutAlpha16 := cutout.At(x, y).RGBA()
			solidAlpha := uint32(solidAlpha16 >> 8)
			cutoutAlpha := uint32(cutoutAlpha16 >> 8)
			foreground.A = uint8((solidAlpha*(0xff-cutoutAlpha) + 0x7f) / 0xff)
			target.SetNRGBA(x, y, foreground)
		}
	}
	return encodePNG(target)
}

func buildTemplateLayers(source []byte) ([]byte, []byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(source))
	viewBox := ""
	transform := ""
	var solidPaths []string
	var cutoutPaths []string

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, fmt.Errorf("parse SVG: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		if start.Name.Local == "svg" {
			viewBox = xmlAttribute(start, "viewBox")
		}
		if start.Name.Local == "g" && xmlAttribute(start, "id") == "fox-artwork" {
			transform = xmlAttribute(start, "transform")
		}

		role := xmlAttribute(start, "data-template")
		if role == "" {
			continue
		}
		pathData, err := templatePathData(start)
		if err != nil {
			return nil, nil, err
		}
		switch role {
		case "solid":
			solidPaths = append(solidPaths, pathData)
		case "cutout":
			cutoutPaths = append(cutoutPaths, pathData)
		default:
			return nil, nil, fmt.Errorf("unknown data-template role %q", role)
		}
	}

	if viewBox == "" || transform == "" || len(solidPaths) == 0 || len(cutoutPaths) == 0 {
		return nil, nil, fmt.Errorf("SVG is missing template geometry metadata")
	}

	layer := func(paths []string) []byte {
		return []byte(fmt.Sprintf(
			`<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s"><g transform="%s"><path fill="#ffffff" d="%s"/></g></svg>`,
			viewBox,
			transform,
			strings.Join(paths, " "),
		))
	}
	return layer(solidPaths), layer(cutoutPaths), nil
}

func templatePathData(start xml.StartElement) (string, error) {
	switch start.Name.Local {
	case "path":
		pathData := xmlAttribute(start, "d")
		if pathData == "" {
			return "", fmt.Errorf("template path %q has no geometry", xmlAttribute(start, "id"))
		}
		return pathData, nil
	case "circle":
		cx, err := xmlFloatAttribute(start, "cx")
		if err != nil {
			return "", err
		}
		cy, err := xmlFloatAttribute(start, "cy")
		if err != nil {
			return "", err
		}
		radius, err := xmlFloatAttribute(start, "r")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"M%g %gA%g %g 0 1 0 %g %gA%g %g 0 1 0 %g %gZ",
			cx-radius,
			cy,
			radius,
			radius,
			cx+radius,
			cy,
			radius,
			radius,
			cx-radius,
			cy,
		), nil
	default:
		return "", fmt.Errorf("unsupported template geometry element %q", start.Name.Local)
	}
}

func xmlAttribute(start xml.StartElement, name string) string {
	for _, attribute := range start.Attr {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func xmlFloatAttribute(start xml.StartElement, name string) (float64, error) {
	value := xmlAttribute(start, name)
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"parse %s %q on template element %q: %w",
			name,
			value,
			xmlAttribute(start, "id"),
			err,
		)
	}
	return parsed, nil
}

func addStatusDot(sourcePNG []byte, size int, statusColor color.NRGBA) ([]byte, error) {
	decoded, err := png.Decode(bytes.NewReader(sourcePNG))
	if err != nil {
		return nil, fmt.Errorf("decode base PNG: %w", err)
	}
	if decoded.Bounds().Dx() != size || decoded.Bounds().Dy() != size {
		return nil, fmt.Errorf(
			"base PNG dimensions = %dx%d, want %dx%d",
			decoded.Bounds().Dx(),
			decoded.Bounds().Dy(),
			size,
			size,
		)
	}

	target := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(target, target.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	mask := image.NewAlpha(target.Bounds())
	center := float64(size) * statusDotCenter
	radius := float64(size) * statusDotDiameter / 2
	const samplesPerAxis = 8
	const sampleCount = samplesPerAxis * samplesPerAxis

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			inside := 0
			for sampleY := 0; sampleY < samplesPerAxis; sampleY++ {
				pointY := float64(y) + (float64(sampleY)+0.5)/samplesPerAxis
				for sampleX := 0; sampleX < samplesPerAxis; sampleX++ {
					pointX := float64(x) + (float64(sampleX)+0.5)/samplesPerAxis
					if math.Hypot(pointX-center, pointY-center) <= radius {
						inside++
					}
				}
			}
			if inside > 0 {
				mask.SetAlpha(x, y, color.Alpha{A: uint8(math.Round(255 * float64(inside) / sampleCount))})
			}
		}
	}

	draw.DrawMask(
		target,
		target.Bounds(),
		&image.Uniform{C: statusColor},
		image.Point{},
		mask,
		image.Point{},
		draw.Over,
	)
	return encodePNG(target)
}

func encodePNG(source image.Image) ([]byte, error) {
	var output bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&output, source); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func encodeICO(images map[int][]byte) []byte {
	var output bytes.Buffer
	_ = binary.Write(&output, binary.LittleEndian, uint16(0))
	_ = binary.Write(&output, binary.LittleEndian, uint16(1))
	_ = binary.Write(&output, binary.LittleEndian, uint16(len(icoSizes)))

	offset := uint32(6 + len(icoSizes)*16)
	for _, size := range icoSizes {
		dimension := byte(size)
		if size == 256 {
			dimension = 0
		}
		output.WriteByte(dimension)
		output.WriteByte(dimension)
		output.WriteByte(0)
		output.WriteByte(0)
		_ = binary.Write(&output, binary.LittleEndian, uint16(1))
		_ = binary.Write(&output, binary.LittleEndian, uint16(32))
		_ = binary.Write(&output, binary.LittleEndian, uint32(len(images[size])))
		_ = binary.Write(&output, binary.LittleEndian, offset)
		offset += uint32(len(images[size]))
	}
	for _, size := range icoSizes {
		output.Write(images[size])
	}
	return output.Bytes()
}

func encodeICNS(images map[int][]byte) []byte {
	chunks := []struct {
		kind string
		size int
	}{
		{kind: "ic07", size: 128},
		{kind: "ic08", size: 256},
		{kind: "ic09", size: 512},
		{kind: "ic10", size: 1024},
	}

	totalSize := 8
	for _, chunk := range chunks {
		totalSize += 8 + len(images[chunk.size])
	}

	var output bytes.Buffer
	output.WriteString("icns")
	_ = binary.Write(&output, binary.BigEndian, uint32(totalSize))
	for _, chunk := range chunks {
		output.WriteString(chunk.kind)
		_ = binary.Write(&output, binary.BigEndian, uint32(8+len(images[chunk.size])))
		output.Write(images[chunk.size])
	}
	return output.Bytes()
}

func encodeWindowsResource(ico []byte) ([]byte, error) {
	icon, err := winres.LoadICO(bytes.NewReader(ico))
	if err != nil {
		return nil, fmt.Errorf("load ICO: %w", err)
	}

	resources := winres.ResourceSet{}
	if err := resources.SetIcon(winres.ID(1), icon); err != nil {
		return nil, fmt.Errorf("set application icon: %w", err)
	}
	resources.SetManifest(winres.AppManifest{
		DPIAwareness: winres.DPIPerMonitorV2,
	})

	var output bytes.Buffer
	if err := resources.WriteObject(&output, winres.ArchAMD64); err != nil {
		return nil, fmt.Errorf("write amd64 COFF object: %w", err)
	}
	return output.Bytes(), nil
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
