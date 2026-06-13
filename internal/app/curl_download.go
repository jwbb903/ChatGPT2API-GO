package app

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const curlImpersonateRepoLatestAPI = "https://api.github.com/repos/lwthiker/curl-impersonate/releases/latest"

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (s *Server) ensureCurlImpersonateBinary() (string, error) {
	if bin := strings.TrimSpace(os.Getenv("CHATGPT2API_CURL_IMPERSONATE_BIN")); bin != "" {
		if st, err := os.Stat(bin); err == nil && !st.IsDir() {
			return bin, nil
		}
		return "", fmt.Errorf("CHATGPT2API_CURL_IMPERSONATE_BIN not found: %s", bin)
	}
	for _, name := range curlImpersonateCandidates() {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	dir := filepath.Join(s.dataDir, "bin", "curl-impersonate")
	if bin := findCurlBinaryInDir(dir); bin != "" {
		return bin, nil
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CHATGPT2API_CURL_IMPERSONATE_AUTO_DOWNLOAD")), "0") || strings.EqualFold(strings.TrimSpace(os.Getenv("CHATGPT2API_CURL_IMPERSONATE_AUTO_DOWNLOAD")), "false") {
		return "", errors.New("curl-impersonate binary not found and auto download disabled")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	assetURL, assetName, err := resolveCurlImpersonateAsset()
	if err != nil {
		return "", err
	}
	archivePath := filepath.Join(dir, assetName)
	if err := downloadFile(archivePath, assetURL); err != nil {
		return "", err
	}
	if err := extractTarGz(archivePath, dir); err != nil {
		return "", err
	}
	_ = os.Remove(archivePath)
	if bin := findCurlBinaryInDir(dir); bin != "" {
		return bin, nil
	}
	return "", fmt.Errorf("curl-impersonate downloaded but no curl binary found in %s", dir)
}

func curlImpersonateCandidates() []string {
	return []string{"curl_edge101", "curl_chrome116", "curl_chrome110", "curl_chrome101", "curl-impersonate", "curl"}
}

func findCurlBinaryInDir(dir string) string {
	for _, name := range curlImpersonateCandidates() {
		matches, _ := filepath.Glob(filepath.Join(dir, "**", name))
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && !st.IsDir() {
				return m
			}
		}
		matches, _ = filepath.Glob(filepath.Join(dir, name))
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && !st.IsDir() {
				return m
			}
		}
	}
	var found string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" {
			return nil
		}
		base := filepath.Base(path)
		for _, name := range curlImpersonateCandidates() {
			if base == name {
				found = path
				return nil
			}
		}
		return nil
	})
	return found
}

func resolveCurlImpersonateAsset() (string, string, error) {
	if explicit := strings.TrimSpace(os.Getenv("CHATGPT2API_CURL_IMPERSONATE_URL")); explicit != "" {
		return explicit, filepath.Base(strings.Split(explicit, "?")[0]), nil
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(curlImpersonateRepoLatestAPI)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("GitHub release API failed: status=%d", resp.StatusCode)
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}
	needle, err := curlAssetNeedle()
	if err != nil {
		return "", "", err
	}
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.HasPrefix(name, "curl-impersonate-") && strings.Contains(name, needle) && strings.HasSuffix(name, ".tar.gz") {
			return asset.URL, asset.Name, nil
		}
	}
	return "", "", fmt.Errorf("no curl-impersonate asset for %s/%s (%s)", runtime.GOOS, runtime.GOARCH, needle)
}

func curlAssetNeedle() (string, error) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		return "x86_64-macos", nil
	}
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("auto download only supports linux and x86_64 macos, current=%s/%s", runtime.GOOS, runtime.GOARCH)
	}
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64-linux-gnu", nil
	case "arm64":
		return "aarch64-linux-gnu", nil
	case "arm":
		return "arm-linux-gnueabihf", nil
	default:
		return "", fmt.Errorf("unsupported linux arch: %s", runtime.GOARCH)
	}
}

func downloadFile(path, url string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func extractTarGz(archivePath, dest string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	base := filepath.Clean(dest)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			continue
		}
		target := filepath.Join(base, name)
		if !strings.HasPrefix(filepath.Clean(target), base) {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)|0755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}
