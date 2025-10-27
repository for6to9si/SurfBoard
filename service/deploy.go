package service

import (
	"SurfBoard/conf"
	"SurfBoard/installer"
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"

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
		installReleaseLive(ctx, ctx.Bot(), *sent, selected.BrowserDownloadURL)
		return nil
	}, th.CallbackDataPrefix("deploy_"))

}

// Асинхронная установка с live логом и фильтрацией wget
func installReleaseLive(ctx context.Context, bot *telego.Bot, msg telego.Message, urlStr string) {
	go func() {
		fileName := getFileNameFromURL(urlStr)
		var logBuilder strings.Builder
		updateInterval := time.Second * 2

		editText(bot, msg, "🚀 *Начинаю установку...*\n")

		// ticker для обновления текста каждые 2 секунды
		ticker := time.NewTicker(updateInterval)
		defer ticker.Stop()
		done := make(chan struct{})

		// фоновая горутина для обновления текста
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
			editText(bot, msg, fmt.Sprintf("❌ Ошибка при обновлении пакетов:\n```\n%s\n```",
				lastLines(logBuilder.String(), 30)))
			close(done)
			return
		}

		// 2️⃣ wget (фильтрация прогресса)
		logBuilder.WriteString(fmt.Sprintf("\n>>> wget %s\n", urlStr))
		if err := runAndLog(&logBuilder, true, "wget", "-O", fileName, urlStr); err != nil {
			editText(bot, msg, fmt.Sprintf("❌ Ошибка при скачивании пакета:\n```\n%s\n```",
				lastLines(logBuilder.String(), 30)))
			close(done)
			return
		}

		// 3️⃣ opkg install
		logBuilder.WriteString(fmt.Sprintf("\n>>> opkg install --force-downgrade %s\n", fileName))
		if err := runAndLog(&logBuilder, false, "opkg", "install", "--force-downgrade", "./"+fileName); err != nil {
			editText(bot, msg, fmt.Sprintf("❌ Ошибка при установке:\n```\n%s\n```",
				lastLines(logBuilder.String(), 30)))
			close(done)
			return
		}

		close(done)
		editText(bot, msg, fmt.Sprintf("✅ *Установка завершена!*\n\n------ Лог ------\n```\n%s\n```",
			lastLines(logBuilder.String(), 40)))
	}()
}

// runAndLog запускает команду и пишет её вывод в лог
// Если filterWget == true — убирает лишние строки из wget
func runAndLog(logBuilder *strings.Builder, filterWget bool, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}

	reader := bufio.NewScanner(io.MultiReader(stdout, stderr))
	for reader.Scan() {
		line := reader.Text()
		if filterWget {
			// Пропускаем прогресс wget (строки с % и точками)
			if strings.Contains(line, "%") || strings.Contains(line, "....") || strings.Contains(line, "K ") {
				continue
			}
		}
		logBuilder.WriteString(line + "\n")
	}
	err := cmd.Wait()
	if err != nil {
		logBuilder.WriteString(fmt.Sprintf("\n⚠️ Ошибка выполнения %s: %v\n", name, err))
	}
	return err
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
