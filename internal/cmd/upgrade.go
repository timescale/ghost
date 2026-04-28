package cmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
)

// binaryFilename is the name of the ghost executable inside a release archive
// (and on disk), with the platform-appropriate extension.
func binaryFilename() string {
	if runtime.GOOS == "windows" {
		return "ghost.exe"
	}
	return "ghost"
}

func buildUpgradeCmd(app *common.App) *cobra.Command {
	var force bool
	var requestedVersion string

	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"update"},
		Short:   "Upgrade the ghost CLI to the latest version",
		Long: `Download and install the latest published version of the ghost CLI, replacing the currently running binary.

If ghost was installed via a package manager (Homebrew, apt, yum/dnf), the upgrade will be refused with a suggestion to use that package manager instead.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(cmd, app, requestedVersion, force)
		},
	}

	cmd.Flags().StringVar(&requestedVersion, "version", "", "specific version to install (e.g. v1.2.3). Defaults to latest.")
	if err := cmd.Flags().MarkHidden("version"); err != nil {
		panic(err)
	}
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if the current version already matches, or the binary was installed via a package manager")
	if err := cmd.Flags().MarkHidden("force"); err != nil {
		panic(err)
	}

	return cmd
}

func runUpgrade(cmd *cobra.Command, app *common.App, requestedVersion string, force bool) error {
	ctx := cmd.Context()
	cfg := app.GetConfig()
	releasesURL := strings.TrimRight(cfg.ReleasesURL, "/")

	// Validate the --version argument up front so we fail fast on bad input
	// without doing any network work.
	if requestedVersion != "" && !semver.IsValid(requestedVersion) {
		return fmt.Errorf("invalid version %q: must be a valid semver version (e.g. v1.2.3)", requestedVersion)
	}

	currentBinaryPath, err := resolveCurrentBinaryPath()
	if err != nil {
		return err
	}

	versionCheckResult, err := common.CheckVersion(ctx, releasesURL)
	if err != nil {
		return fmt.Errorf("failed to check for latest version: %w", err)
	}

	targetVersion := versionCheckResult.LatestVersion
	if requestedVersion != "" {
		targetVersion = requestedVersion
	}
	currentVersion := versionCheckResult.CurrentVersion

	// Package-manager-installed binaries should be upgraded via the package manager.
	switch versionCheckResult.InstallMethod {
	case common.InstallMethodHomebrew, common.InstallMethodDeb, common.InstallMethodRPM:
		if !force {
			return fmt.Errorf("ghost appears to have been installed via %s; upgrade it with:\n    %s",
				versionCheckResult.InstallMethod, versionCheckResult.UpdateCommand)
		}
		cmd.PrintErrf("Warning: ghost appears to have been installed via %s; overwriting from release archive because --force was set\n", versionCheckResult.InstallMethod)
	}

	// Dev builds are typically local, unreleased builds; replacing one with a
	// release archive is almost always surprising, so require --force.
	if currentVersion == "dev" && !force {
		return fmt.Errorf("ghost is a local dev build, not a released version; re-run with --force to replace it with version %s", targetVersion)
	}

	if !force && semver.IsValid(currentVersion) && semver.Compare(targetVersion, currentVersion) == 0 {
		cmd.Printf("ghost is already at version %s\n", currentVersion)
		return nil
	}

	// Verify we can write to the install location before downloading anything.
	if err := checkCanReplaceBinary(currentBinaryPath); err != nil {
		return err
	}

	archiveFilename, archiveIsZip, err := buildReleaseArchiveName()
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "ghost-upgrade-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			cmd.PrintErrf("Warning: failed to clean up temp directory %s: %v\n", tmpDir, removeErr)
		}
	}()

	archivePath := filepath.Join(tmpDir, archiveFilename)
	archiveURL := fmt.Sprintf("%s/releases/%s/%s", releasesURL, targetVersion, archiveFilename)
	checksumURL := archiveURL + ".sha256"

	cmd.Printf("Upgrading ghost %s → %s\n", currentVersion, targetVersion)
	cmd.Printf("Downloading %s\n", archiveURL)
	if err := downloadFile(ctx, archiveURL, archivePath); err != nil {
		return fmt.Errorf("failed to download release archive: %w", err)
	}

	cmd.Println("Verifying checksum")
	expectedChecksum, err := fetchSHA256Checksum(ctx, checksumURL)
	if err != nil {
		return fmt.Errorf("failed to fetch checksum: %w", err)
	}
	if err := verifyFileSHA256(archivePath, expectedChecksum); err != nil {
		return err
	}

	extractedBinaryPath, err := extractBinaryFromArchive(archivePath, archiveIsZip, binaryFilename())
	if err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	cmd.Printf("Installing new binary to %s\n", currentBinaryPath)
	if err := replaceRunningBinary(currentBinaryPath, extractedBinaryPath); err != nil {
		return err
	}

	cmd.Printf("ghost upgraded successfully to %s\n", targetVersion)
	return nil
}

// resolveCurrentBinaryPath returns the absolute path of the running binary,
// resolving any symlinks so that upgrades target the actual file rather than
// replacing a symlink.
func resolveCurrentBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to determine current binary path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// Fall back to the un-resolved path; EvalSymlinks can fail in edge
		// cases (e.g. on some Windows package paths) and we'd still like to
		// attempt the upgrade.
		return exe, nil
	}
	return resolved, nil
}

// buildReleaseArchiveName computes the filename of the release archive for the
// current platform, matching the naming scheme produced by GoReleaser and
// consumed by scripts/install.sh / install.ps1.
//
// The second return value is true for zip archives (Windows) and false for
// tar.gz archives (Linux/macOS).
func buildReleaseArchiveName() (string, bool, error) {
	var osLabel string
	switch runtime.GOOS {
	case "linux":
		osLabel = "Linux"
	case "darwin":
		osLabel = "Darwin"
	case "windows":
		osLabel = "Windows"
	default:
		return "", false, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	var archLabel string
	switch runtime.GOARCH {
	case "amd64":
		archLabel = "x86_64"
	case "arm64":
		archLabel = "arm64"
	case "386":
		archLabel = "i386"
	case "arm":
		archLabel = "armv7"
	default:
		return "", false, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	if runtime.GOOS == "windows" {
		return fmt.Sprintf("ghost_%s_%s.zip", osLabel, archLabel), true, nil
	}
	return fmt.Sprintf("ghost_%s_%s.tar.gz", osLabel, archLabel), false, nil
}

// checkCanReplaceBinary verifies that the process can create files in the
// directory containing the currently running binary, so we fail fast rather
// than downloading a release archive only to discover we lack permission.
func checkCanReplaceBinary(currentBinaryPath string) error {
	parentDir := filepath.Dir(currentBinaryPath)
	probe, err := os.CreateTemp(parentDir, ".ghost-upgrade-writecheck-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s (where ghost is installed): %w\nConsider re-running with elevated privileges, or upgrading via the install method originally used", parentDir, err)
	}
	probePath := probe.Name()
	defer os.Remove(probePath)
	defer probe.Close()

	if err := probe.Close(); err != nil {
		return fmt.Errorf("failed to close write-check probe file %s: %w", probePath, err)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("failed to remove write-check probe file %s: %w", probePath, err)
	}
	return nil
}

// downloadFile downloads the content at url into outputPath.
func downloadFile(ctx context.Context, url, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := api.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, url)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", outputPath, err)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("failed to close response body: %w", err)
	}
	return nil
}

// fetchSHA256Checksum fetches a .sha256 file and returns the hex digest.
// GoReleaser's per-artifact checksum files contain either just the hex digest
// or "<digest>  <filename>" — we accept either.
func fetchSHA256Checksum(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := api.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", errors.New("checksum file is empty")
	}
	if err := resp.Body.Close(); err != nil {
		return "", fmt.Errorf("failed to close response body: %w", err)
	}
	return fields[0], nil
}

// verifyFileSHA256 computes the SHA-256 of filePath and compares with expectedHex.
func verifyFileSHA256(filePath, expectedHex string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}
	actualHex := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHex, expectedHex) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filepath.Base(filePath), expectedHex, actualHex)
	}
	return file.Close()
}

// extractBinaryFromArchive extracts the named binary out of a release archive
// into the archive's parent directory, returning the path to the extracted
// file.
func extractBinaryFromArchive(archivePath string, isZip bool, binaryName string) (string, error) {
	destPath := filepath.Join(filepath.Dir(archivePath), binaryName)
	if isZip {
		return destPath, extractBinaryFromZip(archivePath, binaryName, destPath)
	}
	return destPath, extractBinaryFromTarGz(archivePath, binaryName, destPath)
}

func extractBinaryFromTarGz(archivePath, binaryName, destPath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("binary %q not found in archive", binaryName)
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}
		if err := writeExecutableFile(destPath, tarReader); err != nil {
			return err
		}
		if err := gzReader.Close(); err != nil {
			return fmt.Errorf("failed to close gzip reader: %w", err)
		}
		return file.Close()
	}
}

func extractBinaryFromZip(archivePath, binaryName, destPath string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if filepath.Base(file.Name) != binaryName {
			continue
		}
		if err := copyZipEntryToFile(file, destPath); err != nil {
			return err
		}
		return reader.Close()
	}
	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// copyZipEntryToFile copies the contents of a zip entry into a new executable
// file at destPath.
func copyZipEntryToFile(entry *zip.File, destPath string) error {
	rc, err := entry.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := writeExecutableFile(destPath, rc); err != nil {
		return err
	}
	if err := rc.Close(); err != nil {
		return fmt.Errorf("failed to close zip entry: %w", err)
	}
	return nil
}

func writeExecutableFile(destPath string, src io.Reader) error {
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", destPath, err)
	}
	return nil
}

// replaceRunningBinary replaces the currently executing binary at
// currentBinaryPath with the file at newBinaryPath.
//
// On Linux and macOS the running binary can be overwritten directly because
// the kernel keeps the inode alive while the process runs; an atomic rename
// onto the existing path is safe.
//
// On Windows a running executable cannot be deleted or overwritten, but it
// can be renamed, so we move the existing binary aside (to ghost.exe.old.<pid>)
// before installing the new one. Any accumulated .old.* files from previous
// upgrades are cleaned up opportunistically.
func replaceRunningBinary(currentBinaryPath, newBinaryPath string) error {
	targetDir := filepath.Dir(currentBinaryPath)

	// Stage the new binary in the same directory so the final rename stays
	// on the same filesystem (i.e. is atomic on POSIX).
	stagedFile, err := os.CreateTemp(targetDir, ".ghost-upgrade-staged-*")
	if err != nil {
		return fmt.Errorf("failed to stage new binary in %s: %w", targetDir, err)
	}
	stagedPath := stagedFile.Name()
	// If stagedPath is successfully renamed into place, the deferred Remove
	// becomes a no-op (ENOENT), which we ignore in the defer. Close is
	// deferred as a best-effort safety net on early-return paths; the explicit
	// Close below propagates any real error.
	defer os.Remove(stagedPath)
	defer stagedFile.Close()

	if err := copyFileContents(stagedFile, newBinaryPath); err != nil {
		return err
	}
	if err := stagedFile.Chmod(0o755); err != nil {
		return err
	}
	if err := stagedFile.Close(); err != nil {
		return fmt.Errorf("failed to close staged binary %s: %w", stagedPath, err)
	}

	if runtime.GOOS == "windows" {
		cleanupStaleOldBinaries(currentBinaryPath)

		oldPath := fmt.Sprintf("%s.old.%d", currentBinaryPath, time.Now().UnixNano())
		if err := os.Rename(currentBinaryPath, oldPath); err != nil {
			return fmt.Errorf("failed to move existing binary aside: %w", err)
		}
		if err := os.Rename(stagedPath, currentBinaryPath); err != nil {
			// Try to restore the original so we don't leave the install broken.
			if rollbackErr := os.Rename(oldPath, currentBinaryPath); rollbackErr != nil {
				return fmt.Errorf("failed to install new binary (%w) and failed to restore original from %s: %v", err, oldPath, rollbackErr)
			}
			return fmt.Errorf("failed to install new binary: %w", err)
		}
		// oldPath remains on disk; Windows holds the file open until the
		// current process exits, after which the next upgrade invocation can
		// clean it up.
		return nil
	}

	if err := os.Rename(stagedPath, currentBinaryPath); err != nil {
		return fmt.Errorf("failed to install new binary: %w", err)
	}
	return nil
}

// copyFileContents copies the contents of srcPath into dest (an already-open file).
func copyFileContents(dest *os.File, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	if _, err := io.Copy(dest, src); err != nil {
		return err
	}
	return src.Close()
}

// cleanupStaleOldBinaries removes leftover ghost.exe.old.* files from previous
// Windows upgrades. Files still held open by another process will silently fail
// to delete; that's fine — they'll be cleaned up on a future invocation, so
// Remove errors are intentionally not propagated.
func cleanupStaleOldBinaries(currentBinaryPath string) {
	// filepath.Glob only returns an error for a malformed pattern, which is
	// a programmer error — the pattern we build here is always well-formed.
	matches, err := filepath.Glob(currentBinaryPath + ".old.*")
	if err != nil {
		return
	}
	for _, match := range matches {
		// Best-effort: failure (usually because the file is still locked by
		// another running ghost process) is expected and harmless.
		os.Remove(match) //nolint:errcheck // documented best-effort cleanup
	}
}
