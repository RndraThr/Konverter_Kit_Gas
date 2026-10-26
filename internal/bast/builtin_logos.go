package bast

import (
	"embed"
	"strings"
)

// builtinLogoPrefix marks a program_ba_logo_assets row whose file ships with the app instead of
// living in media storage. Migration 00072 gives every program these rows, so a new program has
// its BA logos without an upload. The key is "builtin:<program_id>/<file>" because storage_key is
// unique across programs.
const builtinLogoPrefix = "builtin:"

//go:embed builtin_logos/*.png
var builtinLogoFS embed.FS

func isBuiltinLogoKey(storageKey string) bool {
	return strings.HasPrefix(storageKey, builtinLogoPrefix)
}

// builtinLogoBytes returns the embedded file a built-in key names.
func builtinLogoBytes(storageKey string) ([]byte, error) {
	name := strings.TrimPrefix(storageKey, builtinLogoPrefix)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || strings.Contains(name, "..") {
		return nil, ErrNotFound
	}
	data, err := builtinLogoFS.ReadFile("builtin_logos/" + name)
	if err != nil {
		return nil, ErrNotFound
	}
	return data, nil
}
