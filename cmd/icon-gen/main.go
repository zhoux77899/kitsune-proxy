// Command icon-gen reproducibly renders platform icons from assets/kitsune.svg.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"github.com/tc-hib/winres"
)

var icoSizes = []int{16, 20, 24, 32, 48, 64, 128, 256}

func main() {
	source, err := os.ReadFile(filepath.Join("assets", "kitsune.svg"))
	if err != nil {
		fatal(err)
	}

	outputDir := filepath.Join("assets", "generated")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fatal(err)
	}

	images := make(map[int][]byte)
	for _, size := range []int{16, 20, 24, 32, 48, 64, 128, 256, 512, 1024} {
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

	if err := os.WriteFile(filepath.Join(outputDir, "kitsune.png"), images[256], 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "kitsune.ico"), ico, 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "kitsune.icns"), icns, 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join("cmd", "kitsune-proxy", "rsrc_windows_amd64.syso"),
		windowsResource,
		0o644,
	); err != nil {
		fatal(err)
	}
}

func renderPNG(source []byte, size int) ([]byte, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(source))
	if err != nil {
		return nil, err
	}
	// oksvg.SetTarget applies a non-zero viewBox translation before scaling.
	// The source icon uses exported canvas coordinates, so compose scale before
	// translation to map the viewBox origin into the target pixel rectangle.
	scaleX := float64(size) / icon.ViewBox.W
	scaleY := float64(size) / icon.ViewBox.H
	icon.Transform = rasterx.Identity.
		Scale(scaleX, scaleY).
		Translate(-icon.ViewBox.X, -icon.ViewBox.Y)

	target := image.NewRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, target, target.Bounds())
	dasher := rasterx.NewDasher(size, size, scanner)
	icon.Draw(dasher, 1)

	var output bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&output, target); err != nil {
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
