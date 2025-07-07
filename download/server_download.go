package download

import (
	"encoding/json"
	"fmt"
	"mcserverlib/types"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
)

func isURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

const ManifestUrl = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

// expandHomeDirectory expands a path starting with "~/" (Unix) or "%USERPROFILE%\" (Windows)
// to the user's home directory. Returns the original path on failure.
func expandHomeDirectory(path string) string {
	if runtime.GOOS == "windows" {
		// Handle %USERPROFILE%\ or ~\ for Windows
		if len(path) > 1 && (path[:2] == "~\\" || path[:2] == "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				return filepath.Join(home, path[2:])
			}
		}
	} else {
		// Unix-like: handle ~/
		if len(path) > 1 && path[:2] == "~/" {
			if home, err := os.UserHomeDir(); err == nil {
				return filepath.Join(home, path[2:])
			}
		}
	}
	return path
}

// DownloadServerJar downloads the Minecraft server JAR file for the specified version.
// It uses caching if enabled and saves the server JAR in the output directory.
func DownloadServerJar(version, outputDirectory string, useCache bool, cacheDirectory string) (string, error) {
	cacheDirPath := expandHomeDirectory(cacheDirectory)
	outputDirectory = filepath.Clean(outputDirectory)

	// Ensure output directory exists
	if err := os.MkdirAll(outputDirectory, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create output directory '%s': %w", outputDirectory, err)
	}

	if isURL(version) {
		// If the version is a direct URL, download it directly
		jarPath := filepath.Join(outputDirectory, "server.jar")
		if err := DownloadFile(version, jarPath, ""); err != nil {
			return "", fmt.Errorf("failed to download server JAR from URL '%s': %w", version, err)
		}

		return jarPath, nil
	} else {
		var manifest *types.VersionManifest
		if useCache {
			manifestPath := filepath.Join(cacheDirPath, "manifest.json")
			if err := os.MkdirAll(cacheDirPath, os.ModePerm); err != nil {
				return "", fmt.Errorf("failed to create cache directory '%s': %w", cacheDirPath, err)
			}
			if err := DownloadFile(ManifestUrl, manifestPath, ""); err != nil {
				return "", fmt.Errorf("failed to download manifest file: %w", err)
			}
			if err := loadJSONFile(manifestPath, &manifest); err != nil {
				return "", fmt.Errorf("failed to parse manifest JSON: %w", err)
			}
		} else {
			var err error
			manifest, err = DownloadJSON[types.VersionManifest](ManifestUrl)
			if err != nil {
				return "", fmt.Errorf("failed to download manifest JSON: %w", err)
			}
		}

		// Resolve the version
		if version == "latest" {
			version = manifest.Latest.Release
		}
		versionEntry, err := findVersion(manifest, version)
		if err != nil {
			return "", err
		}

		// Create mcserverlib directory
		mcserverlibDir := filepath.Join(outputDirectory, ".mcserverlib")
		if err := os.MkdirAll(mcserverlibDir, os.ModePerm); err != nil {
			return "", fmt.Errorf("failed to create mcserverlib directory '%s': %w", mcserverlibDir, err)
		}

		// Download and parse version data
		versionDataPath := filepath.Join(mcserverlibDir, "data.json")
		if err := DownloadFile(versionEntry.URL, versionDataPath, versionEntry.Sha1); err != nil {
			return "", fmt.Errorf("failed to download version data file: %w", err)
		}
		var versionData *types.VersionData
		if err := loadJSONFile(versionDataPath, &versionData); err != nil {
			return "", fmt.Errorf("failed to parse version data JSON: %w", err)
		}

		// Download the server JAR
		jarPath := filepath.Join(outputDirectory, "server.jar")
		if err := DownloadFile(versionData.Downloads.Server.URL, jarPath, versionData.Downloads.Server.Sha1); err != nil {
			return "", fmt.Errorf("failed to download server JAR file: %w", err)
		}

		return jarPath, nil
	}
}

// findVersion searches for a version in the manifest and returns it.
func findVersion(manifest *types.VersionManifest, version string) (*types.Version, error) {
	for _, v := range manifest.Versions {
		if v.ID == version {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("version '%s' not found in manifest", version)
}

// loadJSONFile reads and unmarshals a JSON file into the given target.
func loadJSONFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %w", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal JSON from file '%s': %w", path, err)
	}
	return nil
}