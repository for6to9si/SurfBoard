package service

import (
	"SurfBoard/conf"
	"SurfBoard/installer"
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

var (
	ansiColorRegex   = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	multiSpaceRegex  = regexp.MustCompile(`\s{2,}`)
	controlCharRegex = regexp.MustCompile(`[\t\r]`)
)

func registerDeploy(
	bh *th.BotHandler,
	bot *telego.Bot,
	config *conf.Config,
	isUserAuthorized func(int64) bool,
) {

	// --- Callback "deploy_<имя>" ---
	bh.Handle(func(ctx *th.Context, update telego.Update) error {
		cq := update.CallbackQuery
		if cq.Message == nil || cq.Message.Message == nil {
			// Сообщение недоступно
			_ = ctx.Bot().AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
				CallbackQueryID: cq.ID,
				Text:            "❌ Сообщение недоступно",
			})
			return nil
		}

		data := strings.TrimPrefix(cq.Data, "deploy_")

		idx := strings.LastIndex(data, "_")
		if idx == -1 {
			// На случай, если формат неожиданно другой
			log.Println("unexpected format:", data)
			return nil
		}

		appName := data[:idx]
		version := data[idx+1:]

		var cfg *conf.Programm
		for key, r := range config.Installer.Programs {
			if strings.EqualFold(key, appName) {
				cfg = &r
				break
			}
		}

		versions := installer.RepoConfigs(config.Installer, cfg.Repo)

		var selected *installer.AppLinkButton
		for _, p := range versions {
			if strings.EqualFold(p.Version, version) {
				selected = &p
				break
			}
		}

		if selected == nil {
			return ctx.Bot().AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
				CallbackQueryID: cq.ID,
				Text:            "❌ Программа не найдена",
			})
		}

		rows := make([][]telego.InlineKeyboardButton, 0, len(versions)+1)

		//WithCallbackData(fmt.Sprintf("deploy_%s_%s", sanitizeCallback(appName), selected.Version))

		backBtn := tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("program_" + sanitizeCallback(appName))
		rows = append(rows, tu.InlineKeyboardRow(backBtn))

		// Сразу отвечаем на callback (чтобы Telegram не выдал timeout)
		_ = ctx.Bot().AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: cq.ID,
		})

		// Отправляем сообщение о начале установки
		sent, _ := ctx.Bot().SendMessage(ctx, &telego.SendMessageParams{
			ChatID: tu.ID(cq.Message.GetChat().ID),
			Text:   "🚀 Подготовка к установке...",
		})

		// Запускаем установку в фоне с потоковым логом
		installReleaseLive(ctx.Bot(), cfg, *sent, selected.BrowserDownloadURL)
		return nil
	}, th.CallbackDataPrefix("deploy_"))

}

// Асинхронная установка с live логом, фильтрацией wget и кнопкой возврата
func installReleaseLive(bot *telego.Bot, cfg *conf.Programm, msg telego.Message, urlStr string) {
	go func() {
		fileName := getFileNameFromURL(urlStr)
		var logBuilder strings.Builder
		updateInterval := time.Second * 2

		editText(bot, msg, "🚀 *Начинаю установку...*\n")

		ticker := time.NewTicker(updateInterval)
		defer ticker.Stop()
		done := make(chan struct{})

		// фоновое обновление Telegram-сообщения
		go func() {
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					editText(bot, msg, fmt.Sprintf("📦 Установка...\n```\n%s\n```",
						lastLines(logBuilder.String(), 25)))
				}
			}
		}()

		// 1️⃣ opkg update
		logBuilder.WriteString(">>> opkg update\n")
		if err := RunAndLog(&logBuilder, false, "opkg", "update"); err != nil {
			showError(bot, msg, "Ошибка при обновлении пакетов", logBuilder.String(), done)
			return
		}

		// 2️⃣ wget с фильтрацией вывода
		logBuilder.WriteString(fmt.Sprintf("\n>>> wget %s\n", urlStr))
		if err := RunAndLog(&logBuilder, true, "wget", "-O", fileName, urlStr); err != nil {
			showError(bot, msg, "Ошибка при скачивании пакета", logBuilder.String(), done)
			return
		}

		// 3️⃣ opkg install
		logBuilder.WriteString(fmt.Sprintf("\n>>> opkg install --force-downgrade %s\n", fileName))
		if err := RunAndLog(&logBuilder, false, "opkg", "install", "--force-downgrade", "./"+fileName); err != nil {
			showError(bot, msg, "Ошибка при установке пакета", logBuilder.String(), done)
			return
		}

		// 4️⃣ Очистка кэша после успешной установки
		if err := installer.ClearCache(); err != nil {
			logBuilder.WriteString(fmt.Sprintf("\n⚠️ Ошибка очистки кэша: %v\n", err))
		} else {
			logBuilder.WriteString("\n🧹 Кэш успешно очищен\n")
		}

		// 5️⃣ restart program — теперь несколько команд в cfg.ServiceCommand ([]string)
		if len(cfg.ServiceCommand) > 0 {
			total := len(cfg.ServiceCommand)
			for i, cmdStr := range cfg.ServiceCommand {
				cmdStr = strings.TrimSpace(cmdStr)
				if cmdStr == "" {
					continue // пропустить пустую строку
				}

				// Разбиваем строку на имя команды и аргументы
				parts := strings.Fields(cmdStr)
				if len(parts) == 0 {
					continue
				}
				name := parts[0]
				args := parts[1:]

				// Логируем с номером итерации
				logBuilder.WriteString(fmt.Sprintf("\n>>> [%d/%d] %s %s\n", i+1, total, name, strings.Join(args, " ")))

				if err := RunAndLog(&logBuilder, false, name, args...); err != nil {
					// При ошибке — показываем пользователю накопленный лог и выходим
					showError(bot, msg, fmt.Sprintf("Ошибка при перезапуске (команда %d/%d)", i+1, total), logBuilder.String(), done)
					return
				}

				// Небольшая пауза между командами, чтобы появлялся вывод
				time.Sleep(2 * time.Second)
			}

			logBuilder.WriteString("\n🔁 Все команды перезапуска выполнены успешно\n")

		} else {
			logBuilder.WriteString(fmt.Sprintf("\n>>> Команды перезапуска для программы %s не заданы\n", fileName))
		}

		close(done)

		// 6️⃣ Кнопка "Назад"
		backBtn := tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("manage_apps")
		inlineKb := tu.InlineKeyboard(tu.InlineKeyboardRow(backBtn))

		editMessageWithKeyboard(bot, msg, fmt.Sprintf(
			"✅ *Установка завершена!*\n\n------ Лог ------\n```\n%s\n```",
			lastLines(logBuilder.String(), 40),
		), *inlineKb)
	}()
}

func showError(bot *telego.Bot, msg telego.Message, prefix, log string, done chan struct{}) {
	close(done)
	backBtn := tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("manage_apps")
	inlineKb := tu.InlineKeyboard(tu.InlineKeyboardRow(backBtn))
	editMessageWithKeyboard(bot, msg, fmt.Sprintf("❌ %s:\n```\n%s\n```",
		prefix, lastLines(log, 30)), *inlineKb)
}

// RunAndLog запускает команду и пишет её вывод в лог
// Если filterWget == true — убирает лишние строки из wget
func RunAndLog(logBuilder *strings.Builder, filterWget bool, name string, args ...string) error {
	fullCmd := fmt.Sprintf("%s %s", name, strings.Join(args, " "))

	// Рабочая директория для команды и для временных скриптов (обычно /tmp)
	workDir := os.TempDir()

	// 🧩 Проверяем, обновляется ли SurfBoard через opkg
	if strings.Contains(fullCmd, "opkg install") && strings.Contains(fullCmd, "surfboard") {
		logBuilder.WriteString("\n⚙️ Обнаружено обновление самой программы SurfBoard\n")

		script1 := `/tmp/restart_surfboard.sh`
		script2 := `/tmp/restart_surfboard_fallback.sh`

		restartScript1 := strings.Join([]string{
			"#!/bin/sh",
			"sleep 45",
			"/opt/etc/init.d/S99surfboard start",
			"rm -f " + script1,
			"", // добавляем перевод строки в конце
		}, "\n")

		restartScript2 := strings.Join([]string{
			"#!/bin/sh",
			"sleep 90",
			"/opt/etc/init.d/S99surfboard start",
			"rm -f " + script2,
			"", // добавляет \n в конце
		}, "\n")

		// Попытка записать скрипты в workDir
		if err := os.WriteFile(script1, []byte(restartScript1), 0755); err != nil {
			logBuilder.WriteString(fmt.Sprintf("⚠️ Не удалось записать %s: %v\n", script1, err))
		} else {
			logBuilder.WriteString(fmt.Sprintf("ℹ️ Создан скрипт %s\n", script1))
		}
		if err := os.WriteFile(script2, []byte(restartScript2), 0755); err != nil {
			logBuilder.WriteString(fmt.Sprintf("⚠️ Не удалось записать %s: %v\n", script2, err))
		} else {
			logBuilder.WriteString(fmt.Sprintf("ℹ️ Создан скрипт %s\n", script2))
		}

		// Запускаем их асинхронно из workDir (sh script &)
		if err := exec.Command("sh", "-c", "sh "+script1+" &").Start(); err != nil {
			logBuilder.WriteString(fmt.Sprintf("⚠️ Не удалось запустить %s: %v\n", script1, err))
		}
		if err := exec.Command("sh", "-c", "sh "+script2+" &").Start(); err != nil {
			logBuilder.WriteString(fmt.Sprintf("⚠️ Не удалось запустить %s: %v\n", script2, err))
		}

		logBuilder.WriteString("🚀 Перезапуск SurfBoard будет выполнен через 45 и 90 секунд...\n")
		logBuilder.WriteString("♻️ Текущий процесс завершится для обновления\n")
		logBuilder.WriteString("🎯 Для повторного запуска сервиса воспользуйтесь командой /start\n")
	}

	// 🧰 Обычное выполнение для остальных команд
	cmd := exec.Command(name, args...)
	cmd.Dir = workDir
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return err
	}

	reader := bufio.NewScanner(io.MultiReader(stdout, stderr))
	for reader.Scan() {
		line := cleanLogLine(reader.Text())
		if filterWget {
			if strings.Contains(line, "%") || strings.Contains(line, "....") || strings.Contains(line, "K ") {
				continue
			}
		}
		logBuilder.WriteString(line + "\n")
	}

	if err := cmd.Wait(); err != nil {
		logBuilder.WriteString(fmt.Sprintf("\n⚠️ Ошибка выполнения %s: %v\n", fullCmd, err))
		return err
	}

	return nil
}

func cleanLogLine(line string) string {
	// Убираем ANSI цвета
	line = ansiColorRegex.ReplaceAllString(line, "")

	// Убираем управляющие символы (\t, \r)
	line = controlCharRegex.ReplaceAllString(line, "")

	// Заменяем двойные пробелы
	line = multiSpaceRegex.ReplaceAllString(line, " ")

	// Ставим красивые emoji вместо сообщений
	replacements := map[string]string{
		"отключён":    "❌ отключён",
		"запускается": "🔄 запускается...",
		"запущен":     "🟢 запущен",
		"остановлен":  "🛑 остановлен",
		"ошибка":      "⚠️ ошибка",
	}

	for k, v := range replacements {
		if strings.Contains(strings.ToLower(line), k) {
			return v
		}
	}

	return line
}

// ioMulti объединяет несколько io.Reader
func ioMulti(readers ...io.Reader) io.Reader {
	r, w := io.Pipe()
	go func() {
		for _, rd := range readers {
			sc := bufio.NewScanner(rd)
			for sc.Scan() {
				fmt.Fprintln(w, sc.Text())
			}
		}
		w.Close()
	}()
	return r
}

// getFileNameFromURL извлекает имя файла из URL
func getFileNameFromURL(url string) string {
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

// editText безопасно обновляет сообщение
func editText(bot *telego.Bot, msg telego.Message, text string) {
	_, _ = bot.EditMessageText(
		context.Background(),
		&telego.EditMessageTextParams{
			ChatID:    tu.ID(msg.GetChat().ID),
			MessageID: msg.GetMessageID(),
			Text:      text,
			ParseMode: telego.ModeMarkdown,
		},
	)
}

// lastLines возвращает последние n строк из лога
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func editMessageWithKeyboard(bot *telego.Bot, msg telego.Message, text string, kb telego.InlineKeyboardMarkup) {
	_, _ = bot.EditMessageText(
		context.Background(),
		&telego.EditMessageTextParams{
			ChatID:      tu.ID(msg.GetChat().ID),
			MessageID:   msg.GetMessageID(),
			Text:        text,
			ParseMode:   telego.ModeMarkdown,
			ReplyMarkup: &kb,
		},
	)
}
