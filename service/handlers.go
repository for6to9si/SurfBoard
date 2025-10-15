package service

import (
	"SurfBoard/conf"
	"SurfBoard/installer"
	"fmt"
	"strings"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
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
		case -1:
			status = "🚀 Доступно обновление"
		case 1:
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
				"📊 Состояние: %s",
			selected.App,
			safe(selected.Version),
			safe(selected.Release),
			safe(selected.Path),
			status,
		)

		// Редактируем сообщение безопасно
		_, err := ctx.Bot().EditMessageText(ctx, &telego.EditMessageTextParams{
			ChatID:    tu.ID(cq.Message.GetChat().ID),
			MessageID: cq.Message.GetMessageID(),
			Text:      msg,
			ParseMode: telego.ModeMarkdown,
			ReplyMarkup: tu.InlineKeyboard(
				tu.InlineKeyboardRow(
					tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("manage_apps"),
				),
			),
		})
		// Отвечаем на callback, чтобы убрать "часики"
		_ = ctx.Bot().AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: cq.ID,
		})
		return err
	}, th.CallbackDataPrefix("program_"))

	//bh.Handle(func(ctx *th.Context, update telego.Update) error {
	//	query := update.CallbackQuery
	//	if query == nil || query.Data != "manage_apps" {
	//		return nil
	//	}
	//
	//	loc := locale.Getlocalizer(query.From.LanguageCode)
	//
	//	if !isUserAuthorized(query.From.ID) {
	//		msg, _ := loc.Localize(&i18n.LocalizeConfig{
	//			MessageID: "access_denied",
	//			TemplateData: map[string]interface{}{
	//				"UserID": query.From.ID,
	//			},
	//		})
	//		_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), msg))
	//		_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID))
	//		return nil
	//	}
	//
	//	// Устанавливаем состояние пользователя
	//	user.State = StateSetupApps
	//
	//	// Получаем список программ
	//	programs := installer.GetLocalVersion(config.Installer.Programs)
	//	rows := make([][]telego.InlineKeyboardButton, 0, len(programs)+1)
	//
	//	for _, program := range programs {
	//		var text string
	//		switch {
	//		case !program.Installed:
	//			text = fmt.Sprintf("❌ %s  —  не установлена", program.App)
	//		case program.CompareVersions > 0:
	//			text = fmt.Sprintf("⬆️ %s  •  v%s → %s", program.App, program.Version, program.Release)
	//		case program.CompareVersions == 0:
	//			text = fmt.Sprintf("🟢 %s  •  v%s", program.App, program.Version)
	//		case program.CompareVersions < 0:
	//			text = fmt.Sprintf("⚙️ %s  •  v%s (dev)", program.App, program.Version)
	//		}
	//
	//		btn := tu.InlineKeyboardButton(text).
	//			WithCallbackData("program_" + sanitizeCallback(program.App))
	//		rows = append(rows, tu.InlineKeyboardRow(btn))
	//	}
	//
	//	// Добавляем кнопку "Назад"
	//	backBtn := tu.InlineKeyboardButton("⬅️ Назад").
	//		WithCallbackData("back_to_main")
	//	rows = append(rows, tu.InlineKeyboardRow(backBtn))
	//
	//	// Отправляем сообщение с клавиатурой
	//	_, _ = bot.SendMessage(
	//		ctx,
	//		tu.Message(
	//			tu.ID(query.Message.GetChat().ID),
	//			"📦 Applications:",
	//		).WithReplyMarkup(tu.InlineKeyboard(rows...)),
	//	)
	//
	//	_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID))
	//	return nil
	//}, th.CallbackDataPrefix("manage_apps"))

}

func safe(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
