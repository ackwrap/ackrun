package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/paths"
)

func TestBuildDownloadURLForSupportedPlatforms(t *testing.T) {
	tests := []struct {
		goos string
		arch string
		want string
	}{
		{goos: "windows", arch: "amd64", want: "sing-wrap-1.2.3-windows-amd64.zip"},
		{goos: "linux", arch: "arm64", want: "sing-wrap-1.2.3-linux-arm64-musl.tar.gz"},
		{goos: "darwin", arch: "amd64", want: "sing-wrap-1.2.3-darwin-amd64.tar.gz"},
	}
	for _, tt := range tests {
		url, err := buildDownloadURLFor("1.2.3", tt.goos, tt.arch)
		if err != nil {
			t.Fatalf("build URL for %s/%s: %v", tt.goos, tt.arch, err)
		}
		if !strings.HasSuffix(url, tt.want) {
			t.Fatalf("URL %q does not end with %q", url, tt.want)
		}
	}
	if _, err := buildDownloadURLFor("1.2.3", "plan9", "amd64"); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}

func TestExtractTarGzInstallsRuntimeFiles(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "sing-box.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	writeTarEntry(t, tw, "release/sing-box", "binary", 0600)
	writeTarEntry(t, tw, "release/libcronet.so", "library", 0600)
	writeTarEntry(t, tw, "release/LICENSE", "license", 0600)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	binaryDir := filepath.Join(dir, "bin")
	svc := &InstallerService{paths: &paths.Paths{BinaryDir: binaryDir}}
	if err := svc.extractTarGz(archivePath); err != nil {
		t.Fatalf("extract tar.gz: %v", err)
	}
	info, err := os.Stat(filepath.Join(binaryDir, "sing-box"))
	if err != nil {
		t.Fatalf("stat binary: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0755 {
		t.Fatalf("binary permissions = %o, want 755", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(binaryDir, "libcronet.so")); err != nil {
		t.Fatalf("runtime library missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binaryDir, "LICENSE")); !os.IsNotExist(err) {
		t.Fatalf("non-runtime file should not be installed: %v", err)
	}
}

func TestExtractArchivesRequireCoreBinary(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "missing.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	entry, err := zw.Create("release/LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("license"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	svc := &InstallerService{paths: &paths.Paths{BinaryDir: filepath.Join(dir, "bin")}}
	if err := svc.extractZip(zipPath); err == nil {
		t.Fatal("expected missing core binary error")
	}
}

func TestEnsureCachedDownloadReusesMatchingFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "downloads", "release.zip")
	content := []byte("verified archive")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, content, 0644); err != nil {
		t.Fatal(err)
	}

	called := false
	reused, err := ensureCachedDownload(dest, testSHA256Digest(content), func(string) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("reuse cached download: %v", err)
	}
	if !reused || called {
		t.Fatalf("reused = %v, download called = %v", reused, called)
	}
}

func TestEnsureCachedDownloadReplacesMismatchedFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "downloads", "release.zip")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("stale archive"), 0644); err != nil {
		t.Fatal(err)
	}
	want := []byte("fresh archive")

	reused, err := ensureCachedDownload(dest, testSHA256Digest(want), func(tempPath string) error {
		return os.WriteFile(tempPath, want, 0644)
	})
	if err != nil {
		t.Fatalf("replace cached download: %v", err)
	}
	if reused {
		t.Fatal("mismatched cache should not be reused")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("cached content = %q, want %q", got, want)
	}
}

func TestEnsureCachedDownloadRejectsDownloadedHashMismatch(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "downloads", "release.zip")
	_, err := ensureCachedDownload(dest, testSHA256Digest([]byte("expected")), func(tempPath string) error {
		return os.WriteFile(tempPath, []byte("corrupt"), 0644)
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("unexpected checksum error: %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("invalid download should not be cached: %v", statErr)
	}
}

func TestEnsureCachedDownloadRejectsMissingDigestBeforeDownload(t *testing.T) {
	called := false
	_, err := ensureCachedDownload(filepath.Join(t.TempDir(), "release.zip"), "", func(string) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "missing a SHA-256 digest") {
		t.Fatalf("unexpected digest error: %v", err)
	}
	if called {
		t.Fatal("download should not start without a trusted digest")
	}
}

func testSHA256Digest(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", sum)
}

func writeTarEntry(t *testing.T, tw *tar.Writer, name, content string, mode int64) {
	t.Helper()
	header := &tar.Header{Name: name, Mode: mode, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}
