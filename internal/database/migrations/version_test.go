package migrations

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"testing"
)

var migrationFilenamePattern = regexp.MustCompile(`^(\d+)_.*\.sql$`)

func TestEmbeddedMigrationsUseUniqueVersions(t *testing.T) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}

	filesByVersion := make(map[int][]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationFilenamePattern.FindStringSubmatch(filepath.Base(entry.Name()))
		if matches == nil {
			continue
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil {
			t.Fatalf("parse migration version from %q: %v", entry.Name(), err)
		}
		filesByVersion[version] = append(filesByVersion[version], entry.Name())
	}

	var duplicates []string
	for version, files := range filesByVersion {
		if len(files) < 2 {
			continue
		}
		sort.Strings(files)
		duplicates = append(duplicates, fmt.Sprintf("version %d: %v", version, files))
	}
	sort.Strings(duplicates)
	if len(duplicates) > 0 {
		t.Fatalf("migration versions must be unique; duplicates: %v", duplicates)
	}
}
