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
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
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
		if err := runAndLog(&logBuilder, false, "opkg", "update"); err != nil {
			showError(bot, msg, "Ошибка при обновлении пакетов", logBuilder.String(), done)
			return
		}

		// 2️⃣ wget с фильтрацией вывода
		logBuilder.WriteString(fmt.Sprintf("\n>>> wget %s\n", urlStr))
		if err := runAndLog(&logBuilder, true, "wget", "-O", fileName, urlStr); err != nil {
			showError(bot, msg, "Ошибка при скачивании пакета", logBuilder.String(), done)
			return
		}

		// 3️⃣ opkg install
		logBuilder.WriteString(fmt.Sprintf("\n>>> opkg install --force-downgrade %s\n", fileName))
		if err := runAndLog(&logBuilder, false, "opkg", "install", "--force-downgrade", "./"+fileName); err != nil {
			showError(bot, msg, "Ошибка при установке пакета", logBuilder.String(), done)
			return
		}

		// 4️⃣ Очистка кэша после успешной установки
		if err := installer.ClearCache(); err != nil {
			logBuilder.WriteString(fmt.Sprintf("\n⚠️ Ошибка очистки кэша: %v\n", err))
		} else {
			logBuilder.WriteString("\n🧹 Кэш успешно очищен\n")
		}

		// 5️⃣ restart program
		if cfg.RestartCommand != "" {
			// Разделяем строку на части
			parts := strings.Fields(cfg.RestartCommand)
			// Первый элемент — это имя команды
			name := parts[0]
			// Остальные — аргументы
			args := parts[1:]
			logBuilder.WriteString(fmt.Sprintf("\n>>> %s %s\n", name, strings.Join(args, " ")))

			if err := runAndLog(&logBuilder, false, name, args...); err != nil {
				showError(bot, msg, "Ошибка при перезапуске программы", logBuilder.String(), done)
				return
			}

			// Немного подождать, чтобы успел появиться вывод
			time.Sleep(2 * time.Second)

			logBuilder.WriteString("\n🔁 Программа успешно перезапущена\n")

		} else {
			logBuilder.WriteString(fmt.Sprintf("\n>>> Строка запуска для программы %s не задана \n", fileName))
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

// runAndLog запускает команду и пишет её вывод в лог
// Если filterWget == true — убирает лишние строки из wget
func runAndLog(logBuilder *strings.Builder, filterWget bool, name string, args ...string) error {
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

	// 🧰 Основная часть — запуск команды в PTY
	var cmd *exec.Cmd

	//Использовать “двойной fork” через shell
	if strings.Contains(fullCmd, "S98xray") {
		logBuilder.WriteString("🧩 Обнаружен xray — выполняю без PTY, чтобы избежать SIGHUP\n")
		cmd = exec.Command("sh", "-c", fullCmd)
		cmd.Dir = workDir
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			return err
		}
		reader := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for reader.Scan() {
			logBuilder.WriteString(reader.Text() + "\n")
		}
		if err := cmd.Wait(); err != nil {
			logBuilder.WriteString(fmt.Sprintf("\n⚠️ Ошибка выполнения %s: %v\n", fullCmd, err))
			return err
		}
		return nil
	} else {
		cmd = exec.Command(name, args...)
	}

	// Устанавливаем рабочую директорию для дочернего процесса (чтобы он не пытался писать в RO текущую директорию)
	cmd.Dir = workDir

	// создаём псевдо-терминал
	ptmx, err := pty.Start(cmd)
	if err != nil {
		// Если PTY не создаётся — логируем и пытаемся запустить обычным способом (с той же workDir)
		logBuilder.WriteString(fmt.Sprintf("⚠️ Не удалось создать PTY, выполняю без PTY: %v\n", err))

		// Обычный запуск
		cmd2 := exec.Command(name, args...)
		cmd2.Dir = workDir
		stdout, _ := cmd2.StdoutPipe()
		stderr, _ := cmd2.StderrPipe()
		if err := cmd2.Start(); err != nil {
			return err
		}
		reader := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for reader.Scan() {
			line := reader.Text()
			if filterWget {
				if strings.Contains(line, "%") || strings.Contains(line, "....") || strings.Contains(line, "K ") {
					continue
				}
			}
			logBuilder.WriteString(line + "\n")
		}
		if err := cmd2.Wait(); err != nil {
			logBuilder.WriteString(fmt.Sprintf("\n⚠️ Ошибка выполнения %s: %v\n", fullCmd, err))
			return err
		}
		return nil
	}

	// Закрыть pty в конце
	defer func() {
		logBuilder.WriteString("Закрытие PTY...\n")
		_ = ptmx.Close()
	}()

	// Чтение из PTY
	scanner := bufio.NewScanner(ptmx)
	for scanner.Scan() {
		line := scanner.Text()
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
