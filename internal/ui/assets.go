package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var embeddedAssetVersions sync.Map

var productSplatAssets = map[string]string{
	// leather-handbag.ply in asset-gen is a vase, so it is intentionally mapped
	// only to a vase product.
	"ceramic-vase":     "models/products/ceramic-vase.sog",
	"leather-backpack": "models/products/leather-backpack.sog",
}

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

// ProductSplatURL returns the bundled Gaussian-splat URL for products with a
// matching, reviewed model asset. Splat files use an explicit allowlist rather
// than deriving a path from arbitrary product input.
func ProductSplatURL(productSlug string) string {
	asset, ok := productSplatAssets[strings.TrimSpace(productSlug)]
	if !ok {
		return ""
	}
	if _, err := fs.Stat(StaticFS, "static/"+asset); err != nil {
		return ""
	}
	return AssetURL(asset)
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
