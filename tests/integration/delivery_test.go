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
		if !strings.Contains(workflowText, "cp assets/generated/kitsune.icns") {
			t.Fatalf("%s does not package the macOS application icon", name)
		}
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
		"kitsune.png":  {0x89, 'P', 'N', 'G'},
		"kitsune.ico":  {0, 0, 1, 0},
		"kitsune.icns": {'i', 'c', 'n', 's'},
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
