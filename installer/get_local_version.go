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
	"regexp"
	"sort"
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

func getRelease(url string) (string, error) {

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

func runCommand(ctx context.Context, app string, path string, args []string, url string, timeout time.Duration) VersionInfo {
	info := VersionInfo{
		App:  app,
		Path: path,
		Args: strings.Join(args, " "),
	}

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
	info.Url = url
	info.Release, _ = getRelease(url)
	info.CompareVersions = compareVersions(version, info.Release)
	return info
}

// простое сравнение версий (без учёта префикса v)
func compareVersions(local, remote string) int {
	local = strings.TrimPrefix(local, "v")
	remote = strings.TrimPrefix(remote, "v")

	if local == remote {
		return 0
	}
	return strings.Compare(local, remote)
}

func GetLocalVersion(commands map[string]conf.Programm) []VersionInfo {
	// параллельно выполняем все команды
	ctx := context.Background()
	var wg sync.WaitGroup

	results := make([]VersionInfo, 0, len(commands))
	mu := sync.Mutex{} // защита среза при параллельной записи

	for name, cmd := range commands {
		wg.Add(1)
		go func(app string, path string, args []string, url string) {
			defer wg.Done()
			// таймаут 5 секунд на выполнение каждой команды (можно увеличить)
			info := runCommand(ctx, app, path, args, url, 5*time.Second)
			mu.Lock()
			results = append(results, info)
			mu.Unlock()
		}(name, cmd.ExecutablePath, cmd.Args, cmd.UpdateURL)
	}
	wg.Wait()

	// сортируем срез по названию приложения
	sort.Slice(results, func(i, j int) bool {
		return results[i].App < results[j].App
	})

	return results

}
