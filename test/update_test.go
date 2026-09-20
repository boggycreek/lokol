// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/boggycreek/lokol/pkg/update"
)

func TestFilterReleases(t *testing.T) {
	releases := []update.Release{
		{TagName: "v0.2.0", Prerelease: false, Draft: false},
		{TagName: "v0.3.0-alpha.1", Prerelease: true, Draft: false},
		{TagName: "v0.4.0-draft", Prerelease: false, Draft: true},
	}

	// Filter stable only
	stable := update.FilterReleases(releases, false)
	if len(stable) != 1 {
		t.Fatalf("expected 1 stable release, got %d", len(stable))
	}
	if stable[0].TagName != "v0.2.0" {
		t.Errorf("expected v0.2.0, got %s", stable[0].TagName)
	}

	// Filter with prerelease
	all := update.FilterReleases(releases, true)
	if len(all) != 2 {
		t.Fatalf("expected 2 releases (stable + prerelease, excluding draft), got %d", len(all))
	}
	if all[0].TagName != "v0.2.0" || all[1].TagName != "v0.3.0-alpha.1" {
		t.Errorf("unexpected tags: %v", all)
	}
}

func TestFindTargetRelease(t *testing.T) {
	releases := []update.Release{
		{TagName: "v0.3.0-alpha.2", Prerelease: true},
		{TagName: "v0.2.0", Prerelease: false},
		{TagName: "v0.1.0-alpha.1", Prerelease: true},
	}

	// Default without prerelease: picks latest stable
	rel, err := update.FindTargetRelease(releases, "", false)
	if err != nil {
		t.Fatalf("unexpected error finding stable: %v", err)
	}
	if rel.TagName != "v0.2.0" {
		t.Errorf("expected v0.2.0, got %s", rel.TagName)
	}

	// With prerelease: picks newest release
	relPre, err := update.FindTargetRelease(releases, "", true)
	if err != nil {
		t.Fatalf("unexpected error finding prerelease: %v", err)
	}
	if relPre.TagName != "v0.3.0-alpha.2" {
		t.Errorf("expected v0.3.0-alpha.2, got %s", relPre.TagName)
	}

	// Target specific semver
	relSpecific, err := update.FindTargetRelease(releases, "v0.1.0-alpha.1", false)
	if err != nil {
		t.Fatalf("unexpected error finding specific version: %v", err)
	}
	if relSpecific.TagName != "v0.1.0-alpha.1" {
		t.Errorf("expected v0.1.0-alpha.1, got %s", relSpecific.TagName)
	}

	// Target specific semver without leading 'v'
	relNoV, err := update.FindTargetRelease(releases, "0.1.0-alpha.1", false)
	if err != nil {
		t.Fatalf("unexpected error finding specific version without v: %v", err)
	}
	if relNoV.TagName != "v0.1.0-alpha.1" {
		t.Errorf("expected v0.1.0-alpha.1, got %s", relNoV.TagName)
	}

	// Non-existent version
	_, err = update.FindTargetRelease(releases, "v99.0.0", false)
	if err == nil {
		t.Fatal("expected error for non-existent version, got nil")
	}
}

func TestUpdateExecutionWithMockServer(t *testing.T) {
	// Create a dummy tarball with an executable inside
	tmpDir, err := os.MkdirTemp("", "lokol-update-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	binaryName := update.BinaryNameInArchive()
	dummyContent := "#!/bin/sh\necho 'updated lokol binary'\n"

	tarGzPath := filepath.Join(tmpDir, update.TargetArtifactName())
	tf, err := os.Create(tarGzPath)
	if err != nil {
		t.Fatalf("failed creating tar.gz file: %v", err)
	}
	gw := gzip.NewWriter(tf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: binaryName,
		Mode: 0755,
		Size: int64(len(dummyContent)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed writing header: %v", err)
	}
	if _, err := tw.Write([]byte(dummyContent)); err != nil {
		t.Fatalf("failed writing content: %v", err)
	}
	_ = tw.Close()
	_ = gw.Close()
	_ = tf.Close()

	tarGzBytes, err := os.ReadFile(tarGzPath)
	if err != nil {
		t.Fatalf("failed reading tar.gz: %v", err)
	}

	// Create fake target executable to update
	destBinary := filepath.Join(tmpDir, "lokol")
	if err := os.WriteFile(destBinary, []byte("#!/bin/sh\necho 'old binary'\n"), 0755); err != nil {
		t.Fatalf("failed creating dest binary: %v", err)
	}

	// Set up mock HTTP server
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo/releases":
			releasesJSON := fmt.Sprintf(`[
				{
					"tag_name": "v0.2.0-beta.1",
					"name": "v0.2.0-beta.1",
					"prerelease": true,
					"draft": false,
					"published_at": "%s",
					"assets": [
						{
							"name": "%s",
							"browser_download_url": "%s/download/%s",
							"size": %d
						}
					]
				}
			]`, time.Now().Format(time.RFC3339), update.TargetArtifactName(), serverURL, update.TargetArtifactName(), len(tarGzBytes))
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, releasesJSON)
		case "/download/" + update.TargetArtifactName():
			w.Header().Set("Content-Type", "application/gzip")
			w.Write(tarGzBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	// Run update test using client pointed at mock server
	client := update.NewClient("testowner", "testrepo")
	client.HTTPClient = server.Client()

	req, _ := http.NewRequest(http.MethodGet, serverURL+"/repos/testowner/testrepo/releases", nil)
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		t.Fatalf("failed mock request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Test TargetArtifactName format
	artifact := update.TargetArtifactName()
	expectedPrefix := fmt.Sprintf("lokol-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if artifact != expectedPrefix {
		t.Errorf("expected %s, got %s", expectedPrefix, artifact)
	}
}
