package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var embeddedAssetVersions sync.Map

// AssetURL returns a cache-busted static URL for CSS/JS/images without third-party deps.
func AssetURL(path string) string {
	clean := normalizeAssetPath(path)
	if clean == "" {
		return "/static/"
	}

	if version, ok := localAssetVersion(clean); ok {
		return "/static/" + clean + "?v=" + version
	}

	if version, ok := embeddedAssetVersion(clean); ok {
		return "/static/" + clean + "?v=" + version
	}

	return "/static/" + clean
}

func normalizeAssetPath(path string) string {
	clean := strings.TrimSpace(path)
	clean = strings.TrimPrefix(clean, "/")
	clean = strings.TrimPrefix(clean, "static/")
	return strings.TrimPrefix(clean, "/")
}

func localAssetVersion(cleanPath string) (string, bool) {
	localPath := filepath.Join("internal", "ui", "static", filepath.FromSlash(cleanPath))
	info, err := os.Stat(localPath)
	if err != nil || info.IsDir() {
		return "", false
	}
	return strconv.FormatInt(info.ModTime().Unix(), 10), true
}

func embeddedAssetVersion(cleanPath string) (string, bool) {
	if value, ok := embeddedAssetVersions.Load(cleanPath); ok {
		return value.(string), true
	}

	data, err := StaticFS.ReadFile("static/" + cleanPath)
	if err != nil {
		return "", false
	}

	sum := sha256.Sum256(data)
	version := hex.EncodeToString(sum[:6])
	embeddedAssetVersions.Store(cleanPath, version)
	return version, true
}
