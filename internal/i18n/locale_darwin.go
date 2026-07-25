//go:build darwin

package i18n

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

func detectLocale() string {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	output, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output()
	if err == nil {
		text := string(output)
		for _, line := range strings.Split(text, "\n") {
			line = strings.Trim(strings.TrimSpace(line), "\",()")
			if line != "" {
				return line
			}
		}
	}
	return localeFromEnvironment()
}

func localeFromEnvironment() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}
