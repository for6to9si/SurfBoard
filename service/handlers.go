package service

import (
	"SurfBoard/conf"
	"SurfBoard/installer"
	"SurfBoard/locale"
	"fmt"
	"strings"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// структура ответа GitHub API для последнего релиза
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

func registerHandlers(
	bh *th.BotHandler,
	bot *telego.Bot,
	config *conf.Config,
	isUserAuthorized func(int64) bool,
) {

	// --- Callback "program_<имя>" ---
	bh.Handle(func(ctx *th.Context, update telego.Update) error {

		user := getUserFromUpdate(update)
		if user == nil {
			return nil // нет юзера → ничего не делаем
		}

		loc := locale.Getlocalizer(user.LanguageCode)

		if !isUserAuthorized(user.ID) {
			msg, _ := loc.Localize(&i18n.LocalizeConfig{
				MessageID: "access_denied",
				TemplateData: map[string]interface{}{
					"UserID": user.ID,
				},
			})
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(update.Message.GetChat().ID), msg))
			return nil
		}

		cq := update.CallbackQuery
		if cq.Message == nil || cq.Message.Message == nil {
			// Сообщение недоступно
			_ = ctx.Bot().AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
				CallbackQueryID: cq.ID,
				Text:            "❌ Сообщение недоступно",
			})
			return nil
		}

		appName := strings.TrimPrefix(cq.Data, "program_")
		var selected *installer.VersionInfo
		for _, p := range installer.GetLocalVersion(config.Installer.Programs) {
			if strings.EqualFold(p.App, appName) {
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

		// Формируем статус
		var status string
		switch selected.CompareVersions {
		case 0:
			status = "✅ Установлена последняя версия"
		case 1:
			status = "🚀 Доступно обновление"
		case -1:
			status = "⚙️ Локальная версия новее (dev-сборка)"
		default:
			if !selected.Installed {
				status = "❌ Не установлена"
			} else {
				status = "ℹ️ Статус неизвестен"
			}
		}

		msg := fmt.Sprintf(
			"*%s*\n"+
				"📦 Локальная версия: `%s`\n"+
				"🌐 Последняя версия: `%s`\n"+
				"💾 Путь: `%s`\n"+
				"📊 Состояние: %s\n",
			selected.App,
			safe(selected.Version),
			safe(selected.Release),
			safe(selected.Path),
			status,
		)

		var cfg *conf.Programm
		for key, r := range config.Installer.Programs {
			if strings.EqualFold(key, appName) {
				cfg = &r
				break
			}
		}

		if cfg == nil {
			fmt.Sprintf("❌ Репозиторий %s не найден в конфиге\n", appName)
		}

		versions := installer.RepoConfigs(config.Installer, cfg.Repo)

		rows := make([][]telego.InlineKeyboardButton, 0, len(versions)+1)

		for _, v := range versions {

			var btn telego.InlineKeyboardButton

			btn = tu.InlineKeyboardButton(fmt.Sprintf("%s %s", sanitizeCallback(appName), v.Version)).
				WithCallbackData(fmt.Sprintf("deploy_%s_%s", sanitizeCallback(appName), v.Version))

			rows = append(rows, tu.InlineKeyboardRow(btn))
		}

		backBtn := tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("manage_apps")
		rows = append(rows, tu.InlineKeyboardRow(backBtn))

		msg += "📦 Доступные версии:\n"

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
	}, th.CallbackDataPrefix("program_"))

}

func safe(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// EscapeMarkdownV2 экранирует все специальные символы Telegram MarkdownV2
func EscapeMarkdownV2(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}
