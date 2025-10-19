package installer

import (
	"SurfBoard/conf"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---------- Структуры ----------

type GitHubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type RepoConfig struct {
	Repo         string
	ArchPatterns map[string]*regexp.Regexp
}

type repoJSON struct {
	Repo         string            `json:"repo"`
	ArchPatterns map[string]string `json:"arch_patterns"`
	OnlyRelease  bool              `json:"only_release,omitempty"`
}

// ---------- Константы ----------

const cacheDir = ".cache"
const cacheTTL = 10 * time.Minute

// ---------- Загрузка конфигурации ----------

func LoadRepoConfigs(install conf.Installer) ([]RepoConfig, error) {

	var configs []RepoConfig
	for _, r := range install.Programs {
		compiled := make(map[string]*regexp.Regexp)
		for arch, pattern := range r.ArchPatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("ошибка компиляции regex для %s:%s → %v", r.Repo, arch, err)
			}
			compiled[arch] = re
		}

		cfg := RepoConfig{
			Repo:         r.Repo,
			ArchPatterns: compiled,
		}

		configs = append(configs, cfg)
	}

	return configs, nil
}

// ---------- Работа с кешем ----------

func cachePath(repo string) string {
	file := strings.ReplaceAll(repo, "/", "_") + ".json"
	return filepath.Join(cacheDir, file)
}

func loadFromCache(repo string) ([]GitHubRelease, bool) {
	path := cachePath(repo)
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > cacheTTL {
		return nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var releases []GitHubRelease
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, false
	}

	return releases, true
}

func saveToCache(repo string, releases []GitHubRelease) {
	os.MkdirAll(cacheDir, 0755)
	data, _ := json.MarshalIndent(releases, "", "  ")
	_ = os.WriteFile(cachePath(repo), data, 0644)
}

// ---------- Получение релизов ----------

func fetchReleases(repo string) ([]GitHubRelease, error) {
	// сначала пробуем из кеша
	if cached, ok := loadFromCache(repo); ok {
		fmt.Printf("🟡 %s — загружено из кеша\n", repo)
		return cached, nil
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100", repo)

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "github-release-fetcher")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ошибка GitHub API (%d): %s", resp.StatusCode, body)
	}

	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("ошибка JSON-декода: %v", err)
	}

	saveToCache(repo, releases)
	fmt.Printf("🟢 %s — загружено с GitHub и сохранено в кеш\n", repo)
	return releases, nil
}

// ---------- Основная логика ----------

func fetchLatestForRepo(cfg RepoConfig, maxPerArch int) string {
	releases, err := fetchReleases(cfg.Repo)

	var sb strings.Builder

	if err != nil {
		return fmt.Sprintf("❌ %s: %v\n", cfg.Repo, err)
	}

	sb.WriteString(fmt.Sprintf("\n📦 %s:\n", cfg.Repo))

	arch := "mipsle"

	re := cfg.ArchPatterns[arch]
	var results []string

	for _, release := range releases {
		for _, asset := range release.Assets {
			name := asset.Name
			if !strings.HasSuffix(name, ".ipk") && !strings.HasSuffix(name, ".tar.gz") {
				continue
			}
			if !re.MatchString(name) {
				continue
			}

			results = append(results, fmt.Sprintf("https://github.com/%s/releases/download/%s/%s (v%s)",
				cfg.Repo, release.TagName, name, release.TagName))
		}
	}

	if len(results) == 0 {
		return fmt.Sprintf("❌ %s: %v\n", cfg.Repo, err)
	}

	sort.Slice(results, func(i, j int) bool { return i < j })
	if len(results) > maxPerArch {
		results = results[:maxPerArch]
	}

	sb.WriteString(fmt.Sprintf("  🧩 %s:\n", arch))
	for _, link := range results {
		sb.WriteString(fmt.Sprintf("    %s\n", link))
	}

	return sb.String()
}

func RepoConfigs(install conf.Installer, app conf.Programm) string {
	var sb strings.Builder
	repos, err := LoadRepoConfigs(install)
	if err != nil {
		panic(err)
	}

	var cfg RepoConfig
	for _, r := range repos {
		if r.Repo == app.Repo {
			cfg = r
			break
		}
	}

	sb.WriteString(fetchLatestForRepo(cfg, 3))

	return sb.String()
}
