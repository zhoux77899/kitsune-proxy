//go:build !windows && !darwin

package i18n

import "os"

func detectLocale() string {
	for _, name := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}
