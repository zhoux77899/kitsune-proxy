package integration

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tc-hib/winres"
)

func TestWindowsExecutableContainsApplicationIcon(t *testing.T) {
	root := filepath.Join("..", "..")
	executable := filepath.Join(t.TempDir(), "kitsune-proxy.exe")
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go.exe")

	command := exec.Command(
		goBinary,
		"build",
		"-trimpath",
		"-ldflags",
		"-s -w -H=windowsgui",
		"-o",
		executable,
		"./cmd/kitsune-proxy",
	)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Windows executable: %v\n%s", err, output)
	}

	file, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	resources, err := winres.LoadFromEXE(file)
	if err != nil {
		t.Fatalf("load executable resources: %v", err)
	}
	icon, err := resources.GetIcon(winres.ID(1))
	if err != nil {
		t.Fatalf("load application icon resource: %v", err)
	}

	var encoded bytes.Buffer
	if err := icon.SaveICO(&encoded); err != nil {
		t.Fatalf("encode extracted application icon: %v", err)
	}
	assertICOSizes(t, encoded.Bytes(), []int{16, 20, 24, 32, 48, 64, 128, 256})
}

func assertICOSizes(t *testing.T, ico []byte, want []int) {
	t.Helper()

	if len(ico) < 6 || !bytes.Equal(ico[:4], []byte{0, 0, 1, 0}) {
		t.Fatal("extracted application icon has an invalid ICO header")
	}
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	if count != len(want) || len(ico) < 6+count*16 {
		t.Fatalf("ICO image count = %d, want %d", count, len(want))
	}

	got := make(map[int]bool, count)
	for index := range count {
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
