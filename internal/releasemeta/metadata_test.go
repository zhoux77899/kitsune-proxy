package releasemeta

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func TestGenerateWritesUpdaterManifest(t *testing.T) {
	t.Parallel()

	assetsDir := t.TempDir()
	writeReleaseAssets(t, assetsDir)

	publishedAt := time.Date(2026, time.July, 26, 12, 34, 56, 0, time.UTC)
	if err := Generate(Config{
		AssetsDir:   assetsDir,
		BaseURL:     "https://github.com/zhoux77899/kitsune-proxy/releases/download/v0.1.2",
		Version:     "0.1.2",
		PublishedAt: publishedAt,
	}); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(assetsDir, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got Manifest
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatal(err)
	}

	want := Manifest{
		SchemaVersion: 2,
		Version:       "0.1.2",
		Notes:         "",
		PublishedAt:   "2026-07-26T12:34:56Z",
		Platforms: map[string]UpdateArtifact{
			"windows-amd64": {
				URL:       "https://github.com/zhoux77899/kitsune-proxy/releases/download/v0.1.2/kitsune-proxy-windows-amd64.zip",
				SHA256:    helloSHA256,
				Signature: "",
			},
			"linux-amd64": {
				URL:       "https://github.com/zhoux77899/kitsune-proxy/releases/download/v0.1.2/kitsune-proxy-linux-amd64.tar.gz",
				SHA256:    helloSHA256,
				Signature: "",
			},
			"darwin-amd64": {
				URL:       "https://github.com/zhoux77899/kitsune-proxy/releases/download/v0.1.2/kitsune-proxy-macos-amd64.zip",
				SHA256:    helloSHA256,
				Signature: "",
			},
			"darwin-arm64": {
				URL:       "https://github.com/zhoux77899/kitsune-proxy/releases/download/v0.1.2/kitsune-proxy-macos-arm64.zip",
				SHA256:    helloSHA256,
				Signature: "",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest = %#v, want %#v", got, want)
	}
}

func TestGenerateRejectsUnexpectedBareExecutable(t *testing.T) {
	t.Parallel()

	assetsDir := t.TempDir()
	writeReleaseAssets(t, assetsDir)
	if err := os.WriteFile(
		filepath.Join(assetsDir, "kitsune-proxy-macos-amd64"),
		[]byte("bare executable"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}

	err := Generate(Config{
		AssetsDir:   assetsDir,
		BaseURL:     "https://example.com/releases/download/v0.1.2",
		Version:     "0.1.2",
		PublishedAt: time.Now(),
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected release asset") {
		t.Fatalf("Generate() error = %v, want unexpected release asset", err)
	}
}

func TestGenerateWritesChecksumsForEveryPublishedFile(t *testing.T) {
	t.Parallel()

	assetsDir := t.TempDir()
	writeReleaseAssets(t, assetsDir)
	if err := Generate(Config{
		AssetsDir:   assetsDir,
		BaseURL:     "https://example.com/releases/download/v0.1.2",
		Version:     "0.1.2",
		PublishedAt: time.Date(2026, time.July, 26, 12, 34, 56, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(assetsDir, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	wantNames := []string{
		"kitsune-proxy-linux-amd64.deb",
		"kitsune-proxy-linux-amd64.rpm",
		"kitsune-proxy-linux-amd64.tar.gz",
		"kitsune-proxy-macos-amd64.dmg",
		"kitsune-proxy-macos-amd64.zip",
		"kitsune-proxy-macos-arm64.dmg",
		"kitsune-proxy-macos-arm64.zip",
		"kitsune-proxy-windows-amd64.exe",
		"kitsune-proxy-windows-amd64.zip",
		"latest.json",
	}
	if len(lines) != len(wantNames) {
		t.Fatalf("checksum line count = %d, want %d", len(lines), len(wantNames))
	}
	for index, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != wantNames[index] {
			t.Fatalf("checksum line %d = %q, want asset %q", index, line, wantNames[index])
		}
		asset, err := os.ReadFile(filepath.Join(assetsDir, fields[1]))
		if err != nil {
			t.Fatal(err)
		}
		wantDigest := fmt.Sprintf("%x", sha256.Sum256(asset))
		if fields[0] != wantDigest {
			t.Fatalf("checksum for %s = %s, want %s", fields[1], fields[0], wantDigest)
		}
	}
}

func writeReleaseAssets(t *testing.T, directory string) {
	t.Helper()

	for _, name := range []string{
		"kitsune-proxy-windows-amd64.exe",
		"kitsune-proxy-windows-amd64.zip",
		"kitsune-proxy-linux-amd64.tar.gz",
		"kitsune-proxy-linux-amd64.deb",
		"kitsune-proxy-linux-amd64.rpm",
		"kitsune-proxy-macos-amd64.zip",
		"kitsune-proxy-macos-amd64.dmg",
		"kitsune-proxy-macos-arm64.zip",
		"kitsune-proxy-macos-arm64.dmg",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("hello"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
