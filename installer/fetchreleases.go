package installer

import (
	"SurfBoard/conf"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
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

type AppLinkButton struct {
	BrowserDownloadURL string
	Version            string
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

func fetchLatestForRepo(cfg RepoConfig, maxPerArch int) []AppLinkButton {
	releases, err := fetchReleases(cfg.Repo)
	if err != nil {
		fmt.Printf("❌ %s: %v\n", cfg.Repo, err)
		return nil
	}

	var sb []AppLinkButton

	fmt.Println("Architecture:", runtime.GOARCH)
	fmt.Println("Operating System:", runtime.GOOS)
	getArch()

	arch := "mipsle"
	re := cfg.ArchPatterns[arch]

	for _, release := range releases {

		if release.Prerelease {
			continue
		}

		for _, asset := range release.Assets {
			name := asset.Name
			if !strings.HasSuffix(name, ".ipk") && !strings.HasSuffix(name, ".tar.gz") {
				continue
			}
			if !re.MatchString(name) {
				continue
			}

			sb = append(sb, AppLinkButton{
				BrowserDownloadURL: fmt.Sprintf("https://github.com/%s/releases/download/%s/%s",
					cfg.Repo, release.TagName, name),
				Version: release.TagName,
			})

			// ⛔ Остановимся, если достигли лимита
			if len(sb) >= maxPerArch {
				break
			}
		}

		// выход из внешнего цикла, если уже достигли лимита
		if len(sb) >= maxPerArch {
			break
		}
	}

	// сортируем версии по возрастанию
	sort.Slice(sb, func(i, j int) bool {
		return versionLess(sb[i].Version, sb[j].Version)
	})

	return sb
}

func versionLess(a, b string) bool {
	ap, bp := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(ap), len(bp)); i++ {
		ai, bi := 0, 0
		if i < len(ap) {
			ai, _ = strconv.Atoi(ap[i])
		}
		if i < len(bp) {
			bi, _ = strconv.Atoi(bp[i])
		}
		if ai != bi {
			return ai < bi
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func RepoConfigs(install conf.Installer, repoName string) []AppLinkButton {

	repos, err := LoadRepoConfigs(install)
	if err != nil {
		panic(err)
	}

	var cfg RepoConfig
	for _, r := range repos {
		if r.Repo == repoName {
			cfg = r
			break
		}
	}

	return fetchLatestForRepo(cfg, 3)
}

func getArch() {
	arch := runtime.GOARCH
	fmt.Println("GOARCH:", arch)
	fmt.Println("OS:", runtime.GOOS)

	// Определяем разрядность
	bits := archBits(arch)
	fmt.Printf("Bitness: %d-bit\n", bits)

	// Определяем порядок байт (энднность)
	endianness := detectEndianness(arch)
	fmt.Println("Endianness:", endianness)
}

// Определяет 32 или 64 бит по названию архитектуры
func archBits(arch string) int {
	if strings.Contains(arch, "64") {
		return 64
	}
	return 32
}

// Определяет порядок байт
func detectEndianness(arch string) string {
	// Если архитектура явно указывает "le"
	if strings.HasSuffix(arch, "le") {
		return "Little Endian"
	}
	if strings.HasSuffix(arch, "be") {
		return "Big Endian"
	}

	// Иначе пытаемся определить в рантайме
	var x uint16 = 0xABCD
	b := [2]byte{}
	binary.LittleEndian.PutUint16(b[:], x)
	if b[0] == 0xCD {
		return "Little Endian"
	}
	return "Big Endian"
}
