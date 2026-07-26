// Command release-meta validates release assets and writes latest.json and SHA256SUMS.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/zhoux77899/kitsune-proxy/internal/releasemeta"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("release-meta", flag.ContinueOnError)
	assetsDir := flags.String("assets-dir", "", "directory containing the final release assets")
	baseURL := flags.String("base-url", "", "release download URL without an asset name")
	version := flags.String("version", "", "release version without a v prefix")
	publishedAtText := flags.String("published-at", "", "release publication time in RFC3339 format")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *assetsDir == "" || *baseURL == "" || *version == "" || *publishedAtText == "" {
		return fmt.Errorf("assets-dir, base-url, version, and published-at are required")
	}
	publishedAt, err := time.Parse(time.RFC3339, *publishedAtText)
	if err != nil {
		return fmt.Errorf("parse published-at: %w", err)
	}
	return releasemeta.Generate(releasemeta.Config{
		AssetsDir:   *assetsDir,
		BaseURL:     *baseURL,
		Version:     *version,
		PublishedAt: publishedAt,
	})
}
