package integration

import (
	"bytes"
	"debug/pe"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDeliveryMetadataAndGeneratedIcons(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	for _, name := range []string{"ci.yml", "release.yml"} {
		workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
		if err != nil {
			t.Fatal(err)
		}
		var workflowNode yaml.Node
		if err := yaml.Unmarshal(workflow, &workflowNode); err != nil {
			t.Fatalf("%s workflow YAML: %v", name, err)
		}
		workflowText := string(workflow)
		if !strings.Contains(
			workflowText,
			"git diff --exit-code -- assets/generated cmd/kitsune-proxy/rsrc_windows_amd64.syso",
		) {
			t.Fatalf("%s does not check the generated Windows application resource", name)
		}
	}
	macOSPackager, err := os.ReadFile(filepath.Join(root, "packaging", "macos", "package.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(macOSPackager), "cp assets/generated/kitsune.icns") {
		t.Fatal("shared macOS packager does not package the application icon")
	}

	plist, err := os.ReadFile(filepath.Join(root, "packaging", "macos", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(bytes.NewReader(plist))
	for {
		if _, err := decoder.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("macOS Info.plist XML: %v", err)
		}
	}
	if !strings.Contains(string(plist), "<key>LSUIElement</key>") {
		t.Fatal("macOS Info.plist does not set LSUIElement")
	}
	iconKey := strings.Index(string(plist), "<key>CFBundleIconFile</key>")
	if iconKey < 0 || !strings.Contains(string(plist)[iconKey:], "<string>kitsune</string>") {
		t.Fatal("macOS Info.plist does not reference kitsune.icns")
	}

	signatures := map[string][]byte{
		"kitsune.png":                       {0x89, 'P', 'N', 'G'},
		"kitsune.ico":                       {0, 0, 1, 0},
		"kitsune.icns":                      {'i', 'c', 'n', 's'},
		"kitsune-tray-macos-black.png":      {0x89, 'P', 'N', 'G'},
		"kitsune-tray-macos-white.png":      {0x89, 'P', 'N', 'G'},
		"kitsune-tray-windows-healthy.ico":  {0, 0, 1, 0},
		"kitsune-tray-windows-degraded.ico": {0, 0, 1, 0},
		"kitsune-tray-windows-error.ico":    {0, 0, 1, 0},
		"kitsune-tray-windows-stopped.ico":  {0, 0, 1, 0},
	}
	for name, signature := range signatures {
		content, err := os.ReadFile(filepath.Join(root, "assets", "generated", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(content, signature) {
			t.Fatalf("%s has invalid signature", name)
		}
	}

	resource, err := pe.Open(filepath.Join(
		root,
		"cmd",
		"kitsune-proxy",
		"rsrc_windows_amd64.syso",
	))
	if err != nil {
		t.Fatalf("open Windows application icon resource: %v", err)
	}
	defer resource.Close()
	if resource.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		t.Fatalf("Windows resource machine = %#x, want amd64", resource.Machine)
	}

	for _, section := range resource.Sections {
		if strings.HasPrefix(section.Name, ".rsrc") && section.Size > 0 {
			return
		}
	}
	t.Fatal("Windows application resource has no non-empty .rsrc section")
}

func TestLinuxPackageInstallsDesktopApplication(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	desktop, err := os.ReadFile(filepath.Join(root, "packaging", "linux", "kitsune-proxy.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Kitsune Proxy",
		"Exec=/usr/bin/kitsune-proxy",
		"Icon=kitsune-proxy",
		"Terminal=false",
		"Categories=Network;Utility;",
	} {
		if !strings.Contains(string(desktop), entry+"\n") {
			t.Errorf("Linux desktop entry does not contain %q", entry)
		}
	}

	configuration, err := os.ReadFile(filepath.Join(root, "packaging", "linux", "nfpm.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var configurationNode yaml.Node
	if err := yaml.Unmarshal(configuration, &configurationNode); err != nil {
		t.Fatalf("nFPM configuration YAML: %v", err)
	}
	for _, destination := range []string{
		"/usr/bin/kitsune-proxy",
		"/usr/share/applications/kitsune-proxy.desktop",
		"/usr/share/icons/hicolor/256x256/apps/kitsune-proxy.png",
		"/usr/share/doc/kitsune-proxy/README.md",
		"/usr/share/doc/kitsune-proxy/LICENSE",
	} {
		if !strings.Contains(string(configuration), "dst: "+destination) {
			t.Errorf("nFPM configuration does not install %s", destination)
		}
	}
}

func TestDeliveryWorkflowsUseSharedPackagers(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	for _, name := range []string{"ci.yml", "release.yml"} {
		workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, packager := range []string{
			"packaging/windows/package.ps1",
			"packaging/linux/package.sh",
			"packaging/macos/package.sh",
		} {
			if !strings.Contains(string(workflow), packager) {
				t.Errorf("%s does not use shared packager %s", name, packager)
			}
		}
	}

	linuxPackager, err := os.ReadFile(filepath.Join(root, "packaging", "linux", "package.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(linuxPackager), "github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.47.0") {
		t.Fatal("Linux packager does not pin nFPM v2.47.0")
	}
}
