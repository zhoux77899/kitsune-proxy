package logging

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRotateWriterBoundsBackups(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "kitsune.log")
	writer, err := newRotateWriter(path, 20, 2)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 8; index++ {
		if _, err := writer.Write([]byte("0123456789\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	for _, suffix := range []string{"", ".1", ".2"} {
		if _, err := os.Stat(path + suffix); err != nil {
			t.Fatalf("expected %s: %v", suffix, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected third backup: %v", err)
	}
}

func TestRotateWriterConcurrentWrites(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "kitsune.log")
	writer, err := newRotateWriter(path, 1<<20, 1)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for index := 0; index < 10; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for line := 0; line < 100; line++ {
				_, _ = writer.Write([]byte("event=test secret=[REDACTED]\n"))
			}
		}()
	}
	wait.Wait()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(content), "event=test"); got != 1000 {
		t.Fatalf("line count = %d, want 1000", got)
	}
}

func TestRotateWriterStopsSafelyAfterDiskFailure(t *testing.T) {
	t.Parallel()

	failure := make(chan error, 1)
	writer, err := newRotateWriter(
		filepath.Join(t.TempDir(), "kitsune.log"),
		1<<20,
		1,
		func(err error) {
			failure <- err
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.file.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := writer.Write([]byte("cannot be written")); err == nil {
		t.Fatal("first Write() error = nil")
	}
	select {
	case <-failure:
	default:
		t.Fatal("disk failure callback was not invoked")
	}
	if _, err := writer.Write([]byte("must not panic")); err == nil {
		t.Fatal("second Write() error = nil")
	}
}
