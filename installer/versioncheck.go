package installer

import (
	"SurfBoard/conf"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type VersionInfo struct {
	App             string `json:"app"`
	Path            string `json:"path"`
	Args            string `json:"args,omitempty"`
	Version         string `json:"version,omitempty"`
	Commit          string `json:"commit,omitempty"`
	GoVer           string `json:"go_version,omitempty"`
	Raw             string `json:"raw_output,omitempty"`
	Error           string `json:"error,omitempty"`
	Url             string `json:"url,omitempty"`
	Release         string `json:"release,omitempty"`
	CompareVersions int    `json:"compare_versions,omitempty"`
	Installed       bool   `json:"installed"`
}

type AppCommand struct {
	App  string   `json:"App"`  // название приложения (для идентификации)
	Path string   `json:"Path"` // полный путь к исполняемому файлу
	Args []string `json:"Args"` // аргументы командной строки (обычно для получения версии)
	Url  string   `json:"Url"`  //
}

// структура ответа GitHub API для последнего релиза
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// регулярки для разбора
var (
	semverRe     = regexp.MustCompile(`\d+\.\d+\.\d+(?:[-+][A-Za-z0-9\.\-_]+)?`) // 1.2.3, 1.2.3-rc1, 1.2.3+meta
	shortVerRe   = regexp.MustCompile(`\d+\.\d+(?:[-+][A-Za-z0-9\.\-_]+)?`)      // 1.2 (fallback)
	commitRe     = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)                      // commit hash
	goVerRe      = regexp.MustCompile(`go\d+\.\d+(?:\.\d+)?`)                    // go1.25.1 or go1.20
	nameVerRe    = regexp.MustCompile(`(?i)\b(version|version:)\b`)              // to find keywords
	binaryNameRe = regexp.MustCompile(`^[A-Za-z0-9\-_\.]+`)                      // token at line start
)

// parseOutput пытается извлечь версию/коммит/Go-версию из произвольного вывода
func parseOutput(out string) (version, commit, gov string) {
	// 1) Ищем semver
	if m := semverRe.FindString(out); m != "" {
		version = m
	} else if m := shortVerRe.FindString(out); m != "" {
		// fallback — 1.2 style
		version = m
	}
	// 2) commit hash
	if m := commitRe.FindString(out); m != "" {
		commit = m
	}
	// 3) go version
	if m := goVerRe.FindString(out); m != "" {
		gov = m
	}

	// Доп. эвристика: если ничего не найдено, попытаемся взять первый "внятный" токен после имени бинарника
	if version == "" {
		// разберём первую строку
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) > 0 {
			// попробуем найти токен, похожий на версию (например "Xray 25.9.11 (...)" или "youtubeUnblock 1.1.0-1")
			first := strings.TrimSpace(lines[0])
			parts := strings.Fields(first)
			if len(parts) >= 2 {
				// второй токен часто версия
				cand := strings.Trim(parts[1], " ,;()[]")
				// если кандидат содержит цифру — принимаем
				if strings.IndexFunc(cand, func(r rune) bool { return r >= '0' && r <= '9' }) != -1 {
					// strip non alphanum at ends
					cand = strings.Trim(cand, " ,;()[]")
					// accept if it has digits and dots/hyphens
					if semverRe.MatchString(cand) || shortVerRe.MatchString(cand) || regexp.MustCompile(`[0-9]`).MatchString(cand) {
						version = cand
					}
				}
			} else if len(parts) == 1 {
				// может быть "Version: 0.0.8" — тогда ищем ":" и токен после
				if strings.Contains(first, ":") {
					after := strings.TrimSpace(strings.SplitN(first, ":", 2)[1])
					after = strings.Fields(after)[0]
					if after != "" {
						version = after
					}
				}
			}
		}
	}

	return version, commit, gov
}

func getRelease(repo string) (string, error) {
	client := &http.Client{}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	token := conf.GetConfig().Github.Token
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Sprintln("GitHub API rate limit exceeded"), nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Sprintln("401 Unauthorized"), nil
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

func runCommand(ctx context.Context, app string, path string, args []string, repo string, timeout time.Duration) VersionInfo {
	info := VersionInfo{
		App:  app,
		Path: path,
		Args: strings.Join(args, " "),
	}

	info.Release, _ = getRelease(repo)

	// Проверяем наличие исполняемого файла
	if _, err := os.Stat(path); err != nil {
		info.Installed = false
		info.Error = fmt.Sprintf("binary not found: %v", err)
		info.CompareVersions = -2 // спецкод: нет установленной программы
		return info
	}

	info.Installed = true

	// создаём контекст с таймаутом
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	raw := strings.TrimSpace(outBuf.String() + "\n" + errBuf.String())
	info.Raw = strings.TrimSpace(raw)

	if err != nil {
		// если команда вернула ошибку, всё равно попытаемся распарсить вывод
		info.Error = err.Error()
		// но если нет вывода — укажем что пошло не так
		//if info.Raw == "" {
		//	return info
		//}
	}

	version, commit, gov := parseOutput(info.Raw)
	info.Version = version
	info.Commit = commit
	info.GoVer = gov
	info.Url = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	info.CompareVersions = compareVersions(version, info.Release)
	return info
}

// compareVersionsNum возвращает -1, 0, +1
func compareVersions(local, remote string) int {
	l := convertVersionToNumber(local)
	r := convertVersionToNumber(remote)

	if l < r {
		return 1
	}
	if l > r {
		return -1
	}
	return 0
}

// convertVersionToNumber преобразует строку "1.12.10" → 1012010
// Работает корректно с префиксом "v" и отсутствующими частями.
func convertVersionToNumber(ver string) int64 {
	ver = strings.TrimPrefix(ver, "v")
	parts := strings.Split(ver, ".")

	var nums [3]int64
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.ParseInt(parts[i], 10, 64)
		nums[i] = n
	}

	// 1.12.10 → 1*1_000_000 + 12*1_000 + 10 = 1012010
	return nums[0]*1_000_000 + nums[1]*1_000 + nums[2]
}

func GetLocalVersion(commands map[string]conf.Programm) []VersionInfo {

	// 1️⃣ Проверяем кэш
	if cached, ok := loadCache(); ok {
		fmt.Println("📦 Используется кэш версий (моложе 15 минут)")
		return cached
	}

	// 2️⃣ Если кэша нет — делаем обычный запрос
	// параллельно выполняем все команды
	ctx := context.Background()
	var wg sync.WaitGroup

	results := make([]VersionInfo, 0, len(commands))
	mu := sync.Mutex{} // защита среза при параллельной записи

	for name, cmd := range commands {
		wg.Add(1)
		go func(app string, path string, args []string, repo string) {
			defer wg.Done()
			// таймаут 5 секунд на выполнение каждой команды (можно увеличить)
			info := runCommand(ctx, app, path, args, repo, 5*time.Second)
			mu.Lock()
			results = append(results, info)
			mu.Unlock()
		}(name, cmd.ExecutablePath, cmd.Args, cmd.Repo)
	}
	wg.Wait()

	// сортируем срез по названию приложения
	sort.Slice(results, func(i, j int) bool {
		return results[i].App < results[j].App
	})

	// 3️⃣ Сохраняем в кэш
	saveCache(results)
	fmt.Println("💾 Кэш версий обновлён", getCacheFilePath())

	return results

}

// ---------- Кэширование результатов ----------

var cacheFile = filepath.Join(cacheDir, "version_cache.json")

//const cacheTTL = 15 * time.Minute

// getCacheFilePath возвращает полный путь к файлу кэша
func getCacheFilePath() string {
	cfg := conf.GetConfig()
	cacheDir := cfg.CachePath
	if cacheDir == "" {
		cacheDir = os.TempDir()
	}

	// создаём каталог, если его нет
	_ = os.MkdirAll(cacheDir, 0o755)
	return fmt.Sprintf("%s/version_cache.json", strings.TrimRight(cacheDir, "/"))
}

// saveCache сохраняет срез VersionInfo в файл
func saveCache(data []VersionInfo) {
	cacheFile := getCacheFilePath()
	file, err := os.Create(cacheFile)
	if err != nil {
		fmt.Println("⚠️ Ошибка записи кэша:", err)
		return
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	err = enc.Encode(struct {
		Timestamp time.Time     `json:"timestamp"`
		Data      []VersionInfo `json:"data"`
	}{
		Timestamp: time.Now(),
		Data:      data,
	})
	if err != nil {
		fmt.Println("⚠️ Ошибка сериализации кэша:", err)
	}
}

// loadCache пытается загрузить данные из кэша, если он не устарел
func loadCache() ([]VersionInfo, bool) {
	cacheFile := getCacheFilePath()
	file, err := os.Open(cacheFile)
	if err != nil {
		return nil, false
	}
	defer file.Close()

	var cached struct {
		Timestamp time.Time     `json:"timestamp"`
		Data      []VersionInfo `json:"data"`
	}

	if err := json.NewDecoder(file).Decode(&cached); err != nil {
		return nil, false
	}

	if time.Since(cached.Timestamp) > cacheTTL {
		return nil, false
	}

	return cached.Data, true
}

// ClearCache удаляет файл кэша версий, если он существует
func ClearCache() error {
	cacheFile := getCacheFilePath()
	if _, err := os.Stat(cacheFile); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("ℹ️ Кэш уже очищен — файл не найден:", cacheFile)
			return nil
		}
		return fmt.Errorf("ошибка проверки файла кэша: %w", err)
	}

	if err := os.Remove(cacheFile); err != nil {
		return fmt.Errorf("ошибка удаления кэша: %w", err)
	}

	fmt.Println("🧹 Кэш успешно очищен:", cacheFile)
	return nil
}
