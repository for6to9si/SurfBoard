package service

import (
	"SurfBoard/conf"
	"SurfBoard/installer"
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"

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

		msg := "📦 Installing:\n"

		msg += installRelease(selected.BrowserDownloadURL)

		// Редактируем сообщение безопасно
		_, err := ctx.Bot().EditMessageText(ctx, &telego.EditMessageTextParams{
			ChatID:      tu.ID(cq.Message.GetChat().ID),
			MessageID:   cq.Message.GetMessageID(),
			Text:        msg,
			ParseMode:   telego.ModeMarkdown,
			ReplyMarkup: tu.InlineKeyboard(rows...),
		})
		// Отвечаем на callback, чтобы убрать "часики"
		_ = ctx.Bot().AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: cq.ID,
		})
		return err
	}, th.CallbackDataPrefix("deploy_"))

}

func installRelease(urlStr string) string {

	// Получаем имя файла из URL
	fileName, err := getFileNameFromURL(urlStr)
	if err != nil {
		return fmt.Sprintf("Ошибка при получении имени файла: %v", err)
	}

	var fullLog bytes.Buffer

	fullLog.WriteString(fmt.Sprintf(">>> Обновление списка пакетов..."))
	if out, err := runCommand("opkg", "update"); err != nil {
		return fmt.Sprintf("Ошибка при обновлении пакетов: %v\n%s", err, out)
	} else {
		fullLog.WriteString(out)
	}

	fullLog.WriteString(fmt.Sprintf(">>> Скачивание пакета:", urlStr))
	if out, err := runCommand("wget", "-O", fileName, urlStr); err != nil {
		return fmt.Sprintf("Ошибка при скачивании файла: %v\n%s", err, out)
	} else {
		fullLog.WriteString(out)
	}

	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		return fmt.Sprintf("Файл %s не найден после скачивания", fileName)
	}

	fullLog.WriteString(fmt.Sprintf(">>> Установка пакета..."))
	if out, err := runCommand("opkg", "install", "--force-downgrade", "./"+fileName); err != nil {
		return fmt.Sprintf("Ошибка при установке пакета: %v\n%s", err, out)
	} else {
		fullLog.WriteString(out)
	}

	fullLog.WriteString("✅ Установка завершена успешно!")
	fullLog.WriteString("------ Полный лог выполнения ------")
	return fullLog.String()
}

// runCommand выполняет команду и возвращает её вывод (stdout + stderr)
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// getFileNameFromURL — извлекает имя файла из URL
func getFileNameFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return path.Base(u.Path), nil
}
