// Package releasemeta generates metadata for published release assets.
package releasemeta

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const manifestFileName = "latest.json"

const checksumsFileName = "SHA256SUMS"

var releaseAssetNames = []string{
	"kitsune-proxy-windows-amd64.exe",
	"kitsune-proxy-windows-amd64.zip",
	"kitsune-proxy-linux-amd64.tar.gz",
	"kitsune-proxy-linux-amd64.deb",
	"kitsune-proxy-linux-amd64.rpm",
	"kitsune-proxy-macos-amd64.zip",
	"kitsune-proxy-macos-amd64.dmg",
	"kitsune-proxy-macos-arm64.zip",
	"kitsune-proxy-macos-arm64.dmg",
}

var updateAssetNames = map[string]string{
	"windows-amd64": "kitsune-proxy-windows-amd64.zip",
	"linux-amd64":   "kitsune-proxy-linux-amd64.tar.gz",
	"darwin-amd64":  "kitsune-proxy-macos-amd64.zip",
	"darwin-arm64":  "kitsune-proxy-macos-arm64.zip",
}

// Config describes one release metadata generation run.
type Config struct {
	AssetsDir   string
	BaseURL     string
	Version     string
	PublishedAt time.Time
}

// Manifest is the stable update metadata contract published as latest.json.
type Manifest struct {
	SchemaVersion int                       `json:"schema_version"`
	Version       string                    `json:"version"`
	Notes         string                    `json:"notes"`
	PublishedAt   string                    `json:"pub_date"`
	Platforms     map[string]UpdateArtifact `json:"platforms"`
}

// UpdateArtifact identifies the canonical update payload for one platform.
type UpdateArtifact struct {
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}

// Generate writes the update manifest for the release assets in Config.AssetsDir.
func Generate(config Config) error {
	if err := validateReleaseAssets(config.AssetsDir); err != nil {
		return err
	}

	baseURL := strings.TrimRight(config.BaseURL, "/")
	platforms := make(map[string]UpdateArtifact, len(updateAssetNames))
	for platform, name := range updateAssetNames {
		digest, err := fileSHA256(filepath.Join(config.AssetsDir, name))
		if err != nil {
			return fmt.Errorf("hash update asset %s: %w", name, err)
		}
		platforms[platform] = UpdateArtifact{
			URL:       baseURL + "/" + name,
			SHA256:    digest,
			Signature: "",
		}
	}

	manifest := Manifest{
		SchemaVersion: 2,
		Version:       config.Version,
		Notes:         "",
		PublishedAt:   config.PublishedAt.UTC().Format(time.RFC3339),
		Platforms:     platforms,
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode update manifest: %w", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(filepath.Join(config.AssetsDir, manifestFileName), content, 0o644); err != nil {
		return fmt.Errorf("write update manifest: %w", err)
	}
	if err := writeChecksums(config.AssetsDir); err != nil {
		return err
	}
	return nil
}

func validateReleaseAssets(directory string) error {
	expected := make(map[string]bool, len(releaseAssetNames))
	for _, name := range releaseAssetNames {
		expected[name] = false
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read release assets: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == manifestFileName || name == checksumsFileName {
			continue
		}
		if _, ok := expected[name]; !ok {
			return fmt.Errorf("unexpected release asset %q", name)
		}
		if entry.IsDir() {
			return fmt.Errorf("release asset %q is not a regular file", name)
		}
		expected[name] = true
	}
	for _, name := range releaseAssetNames {
		if !expected[name] {
			return fmt.Errorf("missing release asset %q", name)
		}
	}
	return nil
}

func writeChecksums(directory string) error {
	names := append([]string(nil), releaseAssetNames...)
	names = append(names, manifestFileName)
	sort.Strings(names)

	var content strings.Builder
	for _, name := range names {
		digest, err := fileSHA256(filepath.Join(directory, name))
		if err != nil {
			return fmt.Errorf("hash published file %s: %w", name, err)
		}
		fmt.Fprintf(&content, "%s  %s\n", digest, name)
	}
	if err := os.WriteFile(
		filepath.Join(directory, checksumsFileName),
		[]byte(content.String()),
		0o644,
	); err != nil {
		return fmt.Errorf("write release checksums: %w", err)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]), nil
}
