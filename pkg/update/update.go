// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package update

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/boggycreek/lokol/pkg/version"
)

// ReleaseAsset represents an asset attached to a GitHub release.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Release represents a GitHub release object from the API.
type Release struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	PublishedAt time.Time      `json:"published_at"`
	Body        string         `json:"body"`
	Assets      []ReleaseAsset `json:"assets"`
}

// Options configures the update command behavior.
type Options struct {
	RepoOwner   string
	RepoName    string
	Prerelease  bool   // Allow unstable / pre-release versions
	Version     string // Specific semver version to install (e.g. "v0.1.0-alpha.1" or "0.1.0-alpha.1")
	ListOnly    bool   // List available releases without updating
	CurrentExec string // Path to current executable (or empty to auto-detect via os.Executable)
}

// Client interacts with GitHub releases API.
type Client struct {
	HTTPClient *http.Client
	RepoOwner  string
	RepoName   string
}

// NewClient creates a new update client with default settings.
func NewClient(owner, repo string) *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		RepoOwner:  owner,
		RepoName:   repo,
	}
}

// FetchReleases queries the GitHub API for releases of the repository.
func (c *Client) FetchReleases() ([]Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", c.RepoOwner, c.RepoName)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create release request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "lokol-updater/"+runtime.GOOS)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed fetching releases from GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub releases JSON: %w", err)
	}

	return releases, nil
}

// FilterReleases filters out draft releases and filters by prerelease flag.
func FilterReleases(releases []Release, allowPrerelease bool) []Release {
	var filtered []Release
	for _, r := range releases {
		if r.Draft {
			continue
		}
		if r.Prerelease && !allowPrerelease {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// FindTargetRelease selects the desired release based on target semver or latest available.
func FindTargetRelease(releases []Release, targetVersion string, allowPrerelease bool) (*Release, error) {
	candidates := FilterReleases(releases, allowPrerelease || targetVersion != "")
	if len(candidates) == 0 {
		if !allowPrerelease {
			return nil, fmt.Errorf("no stable releases found (try with --pre to include unstable pre-releases)")
		}
		return nil, fmt.Errorf("no releases found")
	}

	if targetVersion != "" {
		normalizedTarget := targetVersion
		if !strings.HasPrefix(normalizedTarget, "v") {
			normalizedTarget = "v" + normalizedTarget
		}
		for _, r := range candidates {
			normalizedTag := r.TagName
			if !strings.HasPrefix(normalizedTag, "v") {
				normalizedTag = "v" + normalizedTag
			}
			if normalizedTag == normalizedTarget || r.TagName == targetVersion {
				return &r, nil
			}
		}
		return nil, fmt.Errorf("version '%s' not found among available releases", targetVersion)
	}

	// Default: return the first eligible release (GitHub returns releases sorted newest first)
	return &candidates[0], nil
}

// TargetArtifactName returns the expected tarball name for the current OS and ARCH.
func TargetArtifactName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	return fmt.Sprintf("lokol-%s-%s.tar.gz", goos, goarch)
}

// BinaryNameInArchive returns the expected binary filename inside the tarball.
func BinaryNameInArchive() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	return fmt.Sprintf("lokol-%s-%s", goos, goarch)
}

// Run executes the update command logic.
func Run(opts Options) error {
	if opts.RepoOwner == "" {
		opts.RepoOwner = "boggycreek"
	}
	if opts.RepoName == "" {
		opts.RepoName = "lokol"
	}

	client := NewClient(opts.RepoOwner, opts.RepoName)

	fmt.Println("Fetching releases from GitHub...")
	releases, err := client.FetchReleases()
	if err != nil {
		return err
	}

	if opts.ListOnly {
		PrintReleaseList(releases, version.Version)
		return nil
	}

	target, err := FindTargetRelease(releases, opts.Version, opts.Prerelease)
	if err != nil {
		return err
	}

	targetTag := target.TagName
	currentTag := version.Version

	fmt.Printf("Current version : %s\n", currentTag)
	fmt.Printf("Target release  : %s", targetTag)
	if target.Prerelease {
		fmt.Printf(" (pre-release)")
	}
	fmt.Println()

	if strings.TrimPrefix(currentTag, "v") == strings.TrimPrefix(targetTag, "v") && opts.Version == "" {
		fmt.Println("✓ lokol is already up to date!")
		return nil
	}

	artifactName := TargetArtifactName()
	var downloadURL string
	for _, asset := range target.Assets {
		if asset.Name == artifactName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("release %s does not contain prebuilt binary asset '%s' for %s/%s",
			targetTag, artifactName, runtime.GOOS, runtime.GOARCH)
	}

	execPath := opts.CurrentExec
	if execPath == "" {
		var err error
		execPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("unable to determine current executable path: %w", err)
		}
		execPath, err = filepath.EvalSymlinks(execPath)
		if err != nil {
			return fmt.Errorf("unable to resolve executable symlinks: %w", err)
		}
	}

	fmt.Printf("Downloading %s from %s...\n", artifactName, downloadURL)
	tmpFile, err := os.CreateTemp("", "lokol-update-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if err := downloadAsset(client.HTTPClient, downloadURL, tmpFile); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	_ = tmpFile.Close()

	// Extract binary from tar.gz
	extractedTmp, err := extractBinary(tmpFile.Name(), BinaryNameInArchive())
	if err != nil {
		return fmt.Errorf("failed extracting update artifact: %w", err)
	}
	defer os.Remove(extractedTmp)

	// Replace active binary atomically
	if err := installBinary(extractedTmp, execPath); err != nil {
		return fmt.Errorf("failed to install update to %s: %w", execPath, err)
	}

	fmt.Printf("✓ Successfully updated lokol to %s at: %s\n", targetTag, execPath)
	return nil
}

// PrintReleaseList renders a formatted table of available releases.
func PrintReleaseList(releases []Release, currentVersion string) {
	fmt.Println("Available Releases:")
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("%-18s %-12s %-20s %s\n", "VERSION", "TYPE", "PUBLISHED", "NOTES")
	fmt.Println("--------------------------------------------------------------------------------")

	for _, r := range releases {
		if r.Draft {
			continue
		}
		relType := "stable"
		if r.Prerelease {
			relType = "pre-release"
		}

		tagDisplay := r.TagName
		if strings.TrimPrefix(tagDisplay, "v") == strings.TrimPrefix(currentVersion, "v") {
			tagDisplay += " *"
		}

		pubDate := r.PublishedAt.Format("2006-01-02 15:04")
		if r.PublishedAt.IsZero() {
			pubDate = "unknown"
		}

		title := strings.TrimSpace(r.Name)
		if title == "" || title == r.TagName {
			title = "(no title)"
		}

		fmt.Printf("%-18s %-12s %-20s %s\n", tagDisplay, relType, pubDate, title)
	}
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("(* denotes currently installed version)")
}

func downloadAsset(client *http.Client, url string, dest *os.File) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "lokol-updater/"+runtime.GOOS)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	_, err = io.Copy(dest, resp.Body)
	return err
}

func extractBinary(tarGzPath, expectedName string) (string, error) {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("invalid gzip data: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		baseName := filepath.Base(header.Name)
		if baseName == expectedName || baseName == "lokol" {
			tmpOut, err := os.CreateTemp("", "lokol-bin-*")
			if err != nil {
				return "", err
			}
			defer tmpOut.Close()

			if _, err := io.Copy(tmpOut, tr); err != nil {
				_ = os.Remove(tmpOut.Name())
				return "", err
			}
			if err := os.Chmod(tmpOut.Name(), 0755); err != nil {
				_ = os.Remove(tmpOut.Name())
				return "", err
			}
			return tmpOut.Name(), nil
		}
	}

	return "", fmt.Errorf("binary '%s' not found inside archive", expectedName)
}

func installBinary(srcPath, destPath string) error {
	destDir := filepath.Dir(destPath)
	tmpDest, err := os.CreateTemp(destDir, ".lokol-new-*")
	if err != nil {
		// Fall back to direct copy/rename if destDir temp creation fails
		return directCopy(srcPath, destPath)
	}
	tmpName := tmpDest.Name()
	_ = tmpDest.Close()

	src, err := os.Open(srcPath)
	if err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmpName)
		return err
	}
	_ = dst.Close()

	// Atomic replace
	if err := os.Rename(tmpName, destPath); err != nil {
		_ = os.Remove(tmpName)
		return directCopy(srcPath, destPath)
	}

	return os.Chmod(destPath, 0755)
}

func directCopy(srcPath, destPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return os.Chmod(destPath, 0755)
}
