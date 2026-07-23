package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

const (
	updateGithubRepo   = "multigent/multigent"
	updateGithubAPI    = "https://api.github.com/repos/" + updateGithubRepo + "/releases/latest"
	updateGithubAllAPI = "https://api.github.com/repos/" + updateGithubRepo + "/releases"
	updateDownloadBase = "https://github.com/" + updateGithubRepo + "/releases/download"
	updateGiteeAPI     = "https://gitee.com/api/v5/repos/" + updateGithubRepo + "/releases/latest"
)

type updateChannel string

const (
	updateChannelRelease    updateChannel = "release"
	updateChannelPrerelease updateChannel = "pre-release"
	updateChannelBeta       updateChannel = "beta"
)

var cachedLatestVersion struct {
	version   string
	body      string
	channel   updateChannel
	timestamp time.Time
	mu        sync.RWMutex
}

const versionCheckTTL = time.Hour
const updateNotifyTTL = 24 * time.Hour

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Body       string `json:"body"`
	Prerelease bool   `json:"prerelease"`
}

type updateCheckCache struct {
	Channel       string `json:"channel"`
	LatestVersion string `json:"latestVersion"`
	ReleaseBody   string `json:"releaseBody,omitempty"`
	CheckedAt     string `json:"checkedAt"`
	NotifiedAt    string `json:"notifiedAt,omitempty"`
}

func newCheckUpdateCmd() *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "check-update",
		Short: "Check if a newer version is available",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ch, err := resolveUpdateChannel(channel, cmd.Flags().Changed("pre"), cmd.Flags().Changed("beta"))
			if err != nil {
				return err
			}
			release, err := fetchUpdateRelease(ch)
			if err != nil {
				return err
			}
			if isNewerVersion(release.TagName, version) {
				hint := updateCommandForInstall(ch)
				fmt.Fprintf(os.Stderr, "Update available (%s): %s -> %s\nRun: %s\n", ch, version, release.TagName, hint)
			} else {
				fmt.Printf("Already up to date on %s channel (%s).\n", ch, version)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "update channel: release, pre-release, beta")
	cmd.Flags().Bool("pre", false, "check the pre-release channel")
	cmd.Flags().Bool("beta", false, "check the beta channel")
	return cmd
}

func newUpdateCmd() *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Self-update to the latest version",
		Long: `Updates Multigent to the latest version for the selected channel.

Channels:
  release      Stable releases only. This is the default.
  pre-release  Latest GitHub pre-release, including release candidates.
  beta         Latest beta release tag, for early testers.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ch, err := resolveUpdateChannel(channel, cmd.Flags().Changed("pre"), cmd.Flags().Changed("beta"))
			if err != nil {
				return err
			}
			return runSelfUpdate(ch)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "update channel: release, pre-release, beta")
	cmd.Flags().Bool("pre", false, "update from the pre-release channel")
	cmd.Flags().Bool("beta", false, "update from the beta channel")
	return cmd
}

func runSelfUpdate(channel updateChannel) error {
	fmt.Printf("multigent %s\n", version)
	fmt.Printf("Checking for updates on %s channel...\n", channel)

	release, err := fetchUpdateRelease(channel)
	if err != nil {
		return fmt.Errorf("error checking updates: %w", err)
	}

	latest := release.TagName
	if !isNewerVersion(latest, version) {
		fmt.Printf("Already up to date (%s >= %s).\n", version, latest)
		return nil
	}

	label := latest
	if release.Prerelease {
		label += " (pre-release)"
	}
	fmt.Printf("New version available: %s -> %s\n", version, label)

	if method := detectInstallMethod(); method == "brew" || method == "npm" {
		fmt.Println("This installation is managed by a package manager.")
		fmt.Printf("Run: %s\n", updateCommandForInstall(channel))
		return nil
	}

	archiveAsset := updateArchiveAssetName(latest)
	archiveURL := fmt.Sprintf("%s/%s/%s", updateDownloadBase, latest, archiveAsset)
	fmt.Printf("Downloading %s ...\n", archiveURL)

	tmpFile, err := updateDownloadToTemp(archiveURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpFile)

	multigentTmp, err := updateExtractBinary(tmpFile, archiveAsset, "multigent")
	if err != nil {
		return fmt.Errorf("extract multigent failed: %w", err)
	}
	defer os.Remove(multigentTmp)

	mgaTmp, err := updateExtractBinary(tmpFile, archiveAsset, "mga")
	if err != nil {
		return fmt.Errorf("extract mga failed: %w", err)
	}
	defer os.Remove(mgaTmp)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate current binary: %w", err)
	}

	if err := updateReplaceExecutable(execPath, multigentTmp); err != nil {
		return fmt.Errorf("update multigent failed: %w", err)
	}
	mgaPath := filepath.Join(filepath.Dir(execPath), executableName("mga"))
	if _, err := os.Stat(mgaPath); err == nil {
		if err := updateReplaceExecutable(mgaPath, mgaTmp); err != nil {
			return fmt.Errorf("update mga failed: %w", err)
		}
	} else if os.IsNotExist(err) {
		if err := updateCopyFile(mgaTmp, mgaPath); err != nil {
			return fmt.Errorf("install mga failed: %w", err)
		}
		if err := os.Chmod(mgaPath, 0o755); err != nil {
			return fmt.Errorf("chmod mga: %w", err)
		}
	} else {
		return fmt.Errorf("locate mga: %w", err)
	}

	syncNpmVersion(execPath, strings.TrimPrefix(latest, "v"))

	fmt.Printf("Updated to %s\n", latest)
	fmt.Println("Restart multigent to use the new version.")
	return nil
}

// ── async version check ─────────────────────────────────────

func checkUpdateAsync() {
	if version == "dev" || version == "" {
		return
	}
	if updateCheckDisabled() {
		return
	}
	channel, err := resolveUpdateChannel("", false, false)
	if err != nil {
		channel = updateChannelRelease
	}
	go func() {
		release, err := fetchUpdateRelease(channel)
		if err != nil || release == nil || release.TagName == "" {
			return
		}
		cachedLatestVersion.mu.Lock()
		cachedLatestVersion.version = release.TagName
		cachedLatestVersion.body = release.Body
		cachedLatestVersion.channel = channel
		cachedLatestVersion.timestamp = time.Now()
		cachedLatestVersion.mu.Unlock()
		cache := updateCheckCache{
			Channel:       string(channel),
			LatestVersion: release.TagName,
			ReleaseBody:   release.Body,
			CheckedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if existing, ok := readUpdateCheckCache(); ok && existing.Channel == cache.Channel && existing.LatestVersion == cache.LatestVersion {
			cache.NotifiedAt = existing.NotifiedAt
		}
		writeUpdateCheckCache(cache)
	}()
}

// GetCachedUpdateInfo returns cached version check results for the API.
func GetCachedUpdateInfo() (latestVersion, releaseBody string, hasUpdate bool, channelName string, command string) {
	channel, err := resolveUpdateChannel("", false, false)
	if err != nil {
		channel = updateChannelRelease
	}
	command = updateCommandForInstall(channel)
	if version == "dev" || version == "" {
		return "", "", false, string(channel), command
	}
	cachedLatestVersion.mu.RLock()
	ver := cachedLatestVersion.version
	body := cachedLatestVersion.body
	ts := cachedLatestVersion.timestamp
	cachedChannel := cachedLatestVersion.channel
	cachedLatestVersion.mu.RUnlock()

	if ver == "" || cachedChannel != channel || time.Since(ts) > versionCheckTTL {
		if c, ok := readUpdateCheckCache(); ok && c.Channel == string(channel) {
			if checked, err := time.Parse(time.RFC3339, c.CheckedAt); err == nil && time.Since(checked) <= versionCheckTTL {
				if isNewerVersion(c.LatestVersion, version) {
					return c.LatestVersion, c.ReleaseBody, true, string(channel), command
				}
				return c.LatestVersion, "", false, string(channel), command
			}
		}
		checkUpdateAsync()
		return "", "", false, string(channel), command
	}

	if isNewerVersion(ver, version) {
		return ver, body, true, string(channel), command
	}
	return ver, "", false, string(channel), command
}

func maybePrintUpdateReminder(commandPath string) {
	if version == "dev" || version == "" || updateCheckDisabled() {
		return
	}
	if !shouldPrintUpdateReminderForCommand(commandPath) || !isTerminal(os.Stderr) {
		return
	}
	channel, err := resolveUpdateChannel("", false, false)
	if err != nil {
		channel = updateChannelRelease
	}
	cache, ok := readUpdateCheckCache()
	if !ok || cache.Channel != string(channel) || !isNewerVersion(cache.LatestVersion, version) {
		return
	}
	if checked, err := time.Parse(time.RFC3339, cache.CheckedAt); err != nil || time.Since(checked) > 7*24*time.Hour {
		return
	}
	if cache.NotifiedAt != "" {
		if notified, err := time.Parse(time.RFC3339, cache.NotifiedAt); err == nil && time.Since(notified) < updateNotifyTTL {
			return
		}
	}
	fmt.Fprintf(os.Stderr, "\nUpdate available (%s): %s -> %s\nRun: %s\n", channel, version, cache.LatestVersion, updateCommandForInstall(channel))
	cache.NotifiedAt = time.Now().UTC().Format(time.RFC3339)
	writeUpdateCheckCache(cache)
}

// ── fetch helpers ───────────────────────────────────────────

func fetchUpdateRelease(channel updateChannel) (*githubRelease, error) {
	switch channel {
	case updateChannelBeta:
		return fetchLatestBetaRelease()
	case updateChannelPrerelease:
		return fetchLatestPreRelease()
	case updateChannelRelease:
		return fetchLatestStableWithMirror()
	default:
		return nil, fmt.Errorf("unsupported update channel %q", channel)
	}
}

func fetchLatestStableWithMirror() (*githubRelease, error) {
	release, err := fetchLatestStableFromGitee()
	if err == nil && release != nil && release.TagName != "" {
		return release, nil
	}
	return fetchLatestStable()
}

func fetchLatestStableFromGitee() (*githubRelease, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	req, _ := http.NewRequest("GET", updateGiteeAPI, nil)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gitee API returned HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	if release.Prerelease {
		return nil, nil
	}
	return &release, nil
}

func fetchLatestStable() (*githubRelease, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", updateGithubAPI, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var release githubRelease
			if err := json.NewDecoder(resp.Body).Decode(&release); err == nil {
				return &release, nil
			}
		}
	}

	latestURL := "https://github.com/" + updateGithubRepo + "/releases/latest"
	noRedirect := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp2, err := noRedirect.Get(latestURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp2.Body.Close()

	loc := resp2.Header.Get("Location")
	if loc == "" {
		return nil, fmt.Errorf("no release found")
	}
	parts := strings.Split(loc, "/tag/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected redirect: %s", loc)
	}
	return &githubRelease{TagName: parts[1], HTMLURL: loc}, nil
}

func fetchLatestPreRelease() (*githubRelease, error) {
	releases, err := fetchGithubReleases(20)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found")
	}
	for _, release := range releases {
		if release.Prerelease {
			return &release, nil
		}
	}
	return nil, fmt.Errorf("no pre-release found")
}

func fetchLatestBetaRelease() (*githubRelease, error) {
	releases, err := fetchGithubReleases(30)
	if err != nil {
		return nil, err
	}
	for _, release := range releases {
		if release.Prerelease && strings.Contains(strings.ToLower(release.TagName), "beta") {
			return &release, nil
		}
	}
	return nil, fmt.Errorf("no beta release found")
}

func fetchGithubReleases(limit int) ([]githubRelease, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s?per_page=%d", updateGithubAllAPI, limit), nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("parse releases: %w", err)
	}
	return releases, nil
}

// ── version comparison ──────────────────────────────────────

// stripGitDescribe removes the git-describe suffix from a version string.
// "0.2.2-1-g35f23b5-dirty" → "0.2.2", "0.2.2-1-g35f23b5" → "0.2.2", "0.2.2" → "0.2.2".
func stripGitDescribe(v string) string {
	// Pattern: base-N-gHASH[-dirty] where N is commit count and g prefix + hex hash.
	parts := strings.Split(v, "-")
	if len(parts) >= 3 && len(parts[len(parts)-1]) >= 2 {
		// Check for -N-gHASH or -N-gHASH-dirty pattern.
		for i := 1; i < len(parts)-1; i++ {
			isNum := true
			for _, ch := range parts[i] {
				if ch < '0' || ch > '9' {
					isNum = false
					break
				}
			}
			next := parts[i+1]
			isGitHash := len(next) >= 2 && next[0] == 'g'
			if isNum && isGitHash {
				return strings.Join(parts[:i], "-")
			}
		}
	}
	// Also strip trailing "-dirty".
	return strings.TrimSuffix(v, "-dirty")
}

func isNewerVersion(latest, current string) bool {
	if latest == "" || current == "" {
		return false
	}
	if strings.HasPrefix(current, "dev") {
		return true
	}

	l := strings.TrimPrefix(latest, "v")
	c := stripGitDescribe(strings.TrimPrefix(current, "v"))

	lBase, lPre, _ := strings.Cut(l, "-")
	cBase, cPre, _ := strings.Cut(c, "-")

	lParts := strings.Split(lBase, ".")
	cParts := strings.Split(cBase, ".")

	for i := 0; i < len(lParts) || i < len(cParts); i++ {
		var lv, cv int
		if i < len(lParts) {
			_, _ = fmt.Sscanf(lParts[i], "%d", &lv)
		}
		if i < len(cParts) {
			_, _ = fmt.Sscanf(cParts[i], "%d", &cv)
		}
		if lv > cv {
			return true
		}
		if lv < cv {
			return false
		}
	}

	if cPre != "" && lPre == "" {
		return true
	}
	if cPre == "" && lPre != "" {
		return false
	}
	if lPre != "" && cPre != "" {
		return comparePreReleaseSuffix(lPre, cPre) > 0
	}
	return false
}

func comparePreReleaseSuffix(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	max := len(aParts)
	if len(bParts) > max {
		max = len(bParts)
	}
	for i := 0; i < max; i++ {
		var ap, bp string
		if i < len(aParts) {
			ap = aParts[i]
		}
		if i < len(bParts) {
			bp = bParts[i]
		}

		var an, bn int
		aN, _ := fmt.Sscanf(ap, "%d", &an)
		bN, _ := fmt.Sscanf(bp, "%d", &bn)
		aIsNum := aN == 1 && fmt.Sprintf("%d", an) == ap
		bIsNum := bN == 1 && fmt.Sprintf("%d", bn) == bp

		if aIsNum && bIsNum {
			if an != bn {
				return an - bn
			}
			continue
		}
		if ap < bp {
			return -1
		}
		if ap > bp {
			return 1
		}
	}
	return 0
}

// ── download & extract ──────────────────────────────────────

func updateArchiveAssetName(tag string) string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	base := fmt.Sprintf("multigent-%s-%s-%s", tag, goos, goarch)
	if goos == "windows" {
		return base + ".zip"
	}
	return base + ".tar.gz"
}

func updateDownloadToTemp(url string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "multigent-update-*")
	if err != nil {
		return "", err
	}

	size, err := io.Copy(tmp, resp.Body)
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write: %w", err)
	}
	tmp.Close()

	fmt.Printf("Downloaded %.1f MB\n", float64(size)/1024/1024)
	return tmp.Name(), nil
}

func updateExtractBinary(archivePath, archiveName, binary string) (string, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return updateExtractFromZip(archivePath, binary)
	}
	return updateExtractFromTarGz(archivePath, binary)
}

func updateExtractFromTarGz(archivePath, binary string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == binary {
			tmp, err := os.CreateTemp("", "multigent-update-bin-*")
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(tmp, tr); err != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				return "", fmt.Errorf("extract: %w", err)
			}
			tmp.Close()
			return tmp.Name(), nil
		}
	}
	return "", fmt.Errorf("%s not found in archive", binary)
}

func updateExtractFromZip(archivePath, binary string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Base(f.Name) != executableName(binary) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		tmp, err := os.CreateTemp("", "multigent-update-bin-*")
		if err != nil {
			rc.Close()
			return "", err
		}
		if _, err := io.Copy(tmp, rc); err != nil {
			tmp.Close()
			rc.Close()
			os.Remove(tmp.Name())
			return "", fmt.Errorf("extract: %w", err)
		}
		rc.Close()
		tmp.Close()
		return tmp.Name(), nil
	}
	return "", fmt.Errorf("%s not found in archive", binary)
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func updateReplaceExecutable(target, src string) error {
	if err := os.Chmod(src, 0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	backup := target + ".old"
	os.Remove(backup)

	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("backup old binary: %w", err)
	}

	if err := updateCopyFile(src, target); err != nil {
		if restoreErr := os.Rename(backup, target); restoreErr != nil {
			slog.Warn("update: failed to restore old binary after copy error", "error", restoreErr)
		}
		return fmt.Errorf("install new binary: %w", err)
	}

	if err := os.Chmod(target, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}

	os.Remove(backup)
	return nil
}

func updateCopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func syncNpmVersion(execPath, newVer string) {
	binDir := filepath.Dir(execPath)
	if filepath.Base(binDir) != "bin" {
		return
	}
	pkgDir := filepath.Dir(binDir)
	pkgJSON := filepath.Join(pkgDir, "package.json")

	data, err := os.ReadFile(pkgJSON)
	if err != nil {
		return
	}

	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return
	}

	name, _ := pkg["name"].(string)
	if !strings.Contains(name, "multigent") {
		return
	}

	oldVer, _ := pkg["version"].(string)
	oldNorm := strings.TrimPrefix(oldVer, "v")
	newNorm := strings.TrimPrefix(newVer, "v")
	if oldNorm == newNorm {
		return
	}

	pkg["version"] = newVer
	out, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return
	}
	out = append(out, '\n')
	if err := os.WriteFile(pkgJSON, out, 0o644); err != nil {
		slog.Warn("update: failed to sync npm package.json version", "error", err)
		fmt.Println("Note: npm package version not synced. If the next run re-downloads an old version,")
		fmt.Println("  please run: npm update -g @multigent/multigent")
	}
}

func resolveUpdateChannel(flagValue string, preFlag, betaFlag bool) (updateChannel, error) {
	raw := strings.TrimSpace(flagValue)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("MULTIGENT_UPDATE_CHANNEL"))
	}
	if betaFlag {
		raw = string(updateChannelBeta)
	} else if preFlag {
		raw = string(updateChannelPrerelease)
	}
	if raw == "" {
		raw = string(updateChannelRelease)
	}
	raw = strings.ToLower(strings.ReplaceAll(raw, "_", "-"))
	switch raw {
	case "stable", "release", "latest":
		return updateChannelRelease, nil
	case "pre", "prerelease", "pre-release", "rc", "preview":
		return updateChannelPrerelease, nil
	case "beta":
		return updateChannelBeta, nil
	default:
		return "", fmt.Errorf("invalid update channel %q (use release, pre-release, or beta)", raw)
	}
}

func updateCheckDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MULTIGENT_NO_UPDATE_CHECK")))
	return v == "1" || v == "true" || v == "yes"
}

func shouldPrintUpdateReminderForCommand(commandPath string) bool {
	switch commandPath {
	case "", "multigent update", "multigent check-update", "multigent version", "multigent schema":
		return false
	default:
		return true
	}
}

func detectInstallMethod() string {
	execPath, err := os.Executable()
	if err != nil {
		return "binary"
	}
	p := filepath.ToSlash(execPath)
	switch {
	case strings.Contains(p, "/Cellar/multigent/") || strings.Contains(p, "/homebrew/Cellar/multigent/") || strings.Contains(p, "/Homebrew/Cellar/multigent/"):
		return "brew"
	case strings.Contains(p, "/node_modules/@multigent/multigent/"):
		return "npm"
	default:
		return "binary"
	}
}

func updateCommandForInstall(channel updateChannel) string {
	suffix := ""
	if channel != updateChannelRelease {
		suffix = " --channel " + string(channel)
	}
	switch detectInstallMethod() {
	case "brew":
		if channel == updateChannelRelease {
			return "brew update && brew upgrade multigent"
		}
		return "multigent update" + suffix
	case "npm":
		if channel == updateChannelRelease {
			return "npm update -g @multigent/multigent"
		}
		return "multigent update" + suffix
	default:
		return "multigent update" + suffix
	}
}

func updateCheckCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", err
		}
		dir = filepath.Join(home, ".multigent", "cache")
	} else {
		dir = filepath.Join(dir, "multigent")
	}
	return filepath.Join(dir, "update-check.json"), nil
}

func readUpdateCheckCache() (updateCheckCache, bool) {
	path, err := updateCheckCachePath()
	if err != nil {
		return updateCheckCache{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return updateCheckCache{}, false
	}
	var cache updateCheckCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return updateCheckCache{}, false
	}
	return cache, true
}

func writeUpdateCheckCache(cache updateCheckCache) {
	path, err := updateCheckCachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, append(data, '\n'), 0o644)
}
