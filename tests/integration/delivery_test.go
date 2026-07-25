package integration

import (
	"bytes"
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
}
