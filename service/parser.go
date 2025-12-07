package service

import (
	"SurfBoard/benchmarkMode"
	"SurfBoard/conf"
	"SurfBoard/grpcClient"
	"SurfBoard/locale"
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// --- обрабочтик вводимых сообщений ---
func registerParserkHandler(
	bh *th.BotHandler,
	bot *telego.Bot,
	config *conf.Config,
	xraygRpcclient, benchmarkclient *grpcClient.GRpcClient,
	isUserAuthorized func(int64) bool,
) {
	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {

		loc := locale.Getlocalizer(message.From.LanguageCode)

		// Check if user is authorized
		if !isUserAuthorized(message.From.ID) {

			accessDenied, _ := loc.Localize(&i18n.LocalizeConfig{
				MessageID: "access_denied",
				TemplateData: map[string]interface{}{
					"UserID": message.From.ID,
				},
			})

			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(message.Chat.ID),
				accessDenied,
			))
			return nil
		}

		var text string
		// Фильтруем пустые строки
		//var filteredLines []string

		//var tags []string

		switch user.State {
		case StateDefault:
			text = "не выбрано"
			_, _ = bot.SendMessage(ctx, tu.Message(message.Chat.ChatID(), text))
		case StateBenchmark:
			handleVPNState(ctx, message, bot, benchmarkclient, config.BenchmarkSettings.Env)
		case StateXray:
			handleVPNState(ctx, message, bot, xraygRpcclient, config.XwayConf.Env)
		case StateXrayAddDomainToFile:
			handleDomainState(ctx, message, bot, xraygRpcclient, config.XwayConf.Env)
		case StateBenchmarkAddDomainToFile:
			handleDomainState(ctx, message, bot, benchmarkclient, config.BenchmarkSettings.Env)
		case StateSingBox:
			text = "StateSingBox!"
		case StateSetupApps:
			text = "StateSetupApps!"
		default:
			text = "unknown state"
			_, _ = bot.SendMessage(ctx, tu.Message(message.Chat.ChatID(), text))
			panic("unknown state")
		}

		upd := telego.Update{Message: &message}
		return ctx.Next(upd)
	})
}

// 🔧 Универсальная функция обработки состояний Benchmark и Xray
func handleVPNState(
	ctx *th.Context,
	message telego.Message,
	bot *telego.Bot,
	client *grpcClient.GRpcClient,
	Env conf.Env,
) {
	lines := strings.Split(message.Text, "\n")
	trimmedText := strings.TrimSpace(message.Text)

	// Проверка на "set n" или просто "n"
	var indexStr string
	if strings.HasPrefix(trimmedText, "set ") {
		parts := strings.SplitN(trimmedText, " ", 2)
		if len(parts) == 2 {
			indexStr = strings.TrimSpace(parts[1])
		}
	} else if _, err := strconv.Atoi(trimmedText); err == nil {
		// Если введено просто число
		indexStr = trimmedText
	}

	if indexStr != "" {
		x, err := strconv.Atoi(indexStr)
		if err != nil {
			_, _ = bot.SendMessage(ctx, tu.Message(
				message.Chat.ChatID(),
				fmt.Sprintf("Ошибка: `%s` не является числом", indexStr),
			))
			return
		}

		// Получаем все теги
		_, allTags := client.ListVPNStatuses()
		if x < 0 || x >= len(allTags) {
			_, _ = bot.SendMessage(ctx, tu.Message(
				message.Chat.ChatID(),
				fmt.Sprintf("Ошибка: индекс %d вне диапазона (0..%d)", x, len(allTags)-1),
			))
			return
		}

		// Запускаем OverrideBalancerTarget
		grpcClient.OverrideBalancerTarget(client, "bestVPN", allTags[x])
		_, _ = bot.SendMessage(ctx, tu.Message(
			message.Chat.ChatID(),
			fmt.Sprintf("Balancer переопределён на: %s", allTags[x]),
		))
		return
	}

	// Если это не команда, то обрабатываем строки как раньше
	var filteredLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			filteredLines = append(filteredLines, trimmed)
		}
	}

	tags := benchmarkMode.ParsesVpns(filteredLines, Env.XrayLocationConfdir) //обратка vless://..., ss://..

	for _, line := range tags {
		if strings.TrimSpace(line) == "" {
			continue
		}
		_, _ = bot.SendMessage(ctx, tu.Message(message.Chat.ChatID(), line))
	}
}

func ExtractOne(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "\",") // убираем кавычки и запятые вокруг

	if raw == "" {
		return nil
	}

	// 0️⃣ Формат: 0: "domain.com"
	// проверяем: начинается с цифры, двоеточие, кавычка
	if idx := strings.Index(raw, ":"); idx > 0 {
		if raw[0] >= '0' && raw[0] <= '9' {
			// пример: 0: "mail.google.com"
			parts := strings.SplitN(raw, ":", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1]) // "mail.google.com"
				val = strings.Trim(val, "\"")      // mail.google.com
				if strings.Contains(val, ".") {
					return []string{"domain:" + val}
				}
			}
		}
	}

	// 1️⃣ Если начинается с domain:
	if strings.HasPrefix(raw, "domain:") {
		return []string{raw}
	}

	// 2️⃣ Если начинается с ext: — оставить как есть
	if strings.HasPrefix(raw, "ext:") {
		return []string{raw}
	}

	// 3️⃣ Формат geosite без ext: (geosite_v2fly.dat:vmware)
	if strings.Contains(raw, ".dat:") {
		return []string{raw}
	}

	// 4️⃣ Если это URL → извлечь домен
	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
		host := parsed.Host
		// убираем порт
		host = strings.Split(host, ":")[0]
		return []string{"domain:" + host}
	}

	// 4️⃣ Если это доменное имя (без схемы)
	if strings.Contains(raw, ".") {
		raw = strings.Split(raw, ":")[0] // убрать порт (если вдруг)
		return []string{"domain:" + raw}
	}

	// 6️⃣ Ничего не подошло — игнорируем
	return nil
}

func ExtractDomainsAll(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "\",") // убрать кавычки и запятые вокруг

	if line == "" {
		return nil
	}

	// В строке могут быть несколько элементов: "a.com,b.com,domain:x"
	parts := strings.Split(line, ",")

	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\",")
		if part == "" {
			continue
		}

		out = append(out, ExtractOne(part)...)
	}

	return out
}

// 🔧 Универсальная функция обработки состояний Benchmark и Xray
func handleDomainState(
	ctx *th.Context,
	message telego.Message,
	bot *telego.Bot,
	client *grpcClient.GRpcClient,
	Env conf.Env,
) {
	var domains []string

	lines := strings.Split(message.Text, "\n")
	for _, line := range lines {
		domains = append(domains, ExtractDomainsAll(line)...)
	}

	if user.Domainlist == nil || len(user.Domainlist) == 0 {
		str := []string{
			"Продолжает работать некорректно?",
			"Откройте консоль браузера:",
			"1. На проблемной странице нажмите клавишу F12",
			"2. Или кликните правой кнопкой мыши → «Посмотреть код» → вкладка «Console»",
			"3. Скопируйте и вставьте эту команду в консоль, затем нажмите Enter:",
			"<code>[...new Set(performance.getEntriesByType('resource').map(r => (new URL(r.name)).hostname))]</code>",
			"Результат покажет все серверы, к которым обращался сайт.",
		}

		msg := tu.Message(
			message.Chat.ChatID(),
			strings.Join(str, "\n"),
		)

		msg.ParseMode = telego.ModeHTML

		_, _ = bot.SendMessage(ctx, msg)
	}

	user.Domainlist = append(user.Domainlist, domains...)

	results := []string{}

	results = append(results, "Полный список доменов:")
	results = append(results, user.Domainlist...) // распаковка слайса

	pbcf, msgs := client.AddDomainsConf(Env, FileTmpRoutingBalancers, user.Domainlist)
	// Добавляем сообщения
	results = append(results, msgs...)

	msgAnimation, _ := bot.SendMessage(ctx, tu.Message(message.Chat.ChatID(), "⏳ Обрабатываю..."))

	stop := make(chan struct{})
	go animateLoading(bot, message.Chat.ChatID(), msgAnimation.MessageID, stop)

	prcf, msgs := client.AddDomainsRulesBuild(pbcf)

	stop <- struct{}{}
	bot.EditMessageText(ctx, tu.EditMessageText(message.Chat.ChatID(), msgAnimation.MessageID, "✔ Готово!"))
	// Добавляем сообщения
	results = append(results, msgs...)

	results = append(results, client.AddDomainsAddRule(prcf)...)

	// Создаем массив рядов клавиатуры
	rows := [][]telego.InlineKeyboardButton{
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("📄 Сохранить в файле:" + FileTmpRoutingBalancers).WithCallbackData("save_routing_file"),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("⚙️ " + FileSystemDefault).WithCallbackData(FileSystemDefault),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main"), //benchmark_vpn or xray_vpn
		),
	}

	// Попытка удалить сообщение бота (если есть saved ID)
	if user.LastBotMsgID != 0 {
		if err := bot.DeleteMessage(ctx, &telego.DeleteMessageParams{
			ChatID:    tu.ID(message.Chat.ID),
			MessageID: user.LastBotMsgID,
		}); err != nil {
			log.Printf("failed to delete prompt message (id=%d): %v", user.LastBotMsgID, err)
			// не прерываем — всё равно отправим ответ бота
		} else {
			// успешно удалили — можно обнулить
			user.LastBotMsgID = 0
			// persist user если нужно
		}
	}

	// Отправляем новое сообщение бота
	sent, _ := bot.SendMessage(ctx, &telego.SendMessageParams{
		ChatID:      tu.ID(message.Chat.ID),
		Text:        strings.Join(results, "\n"),
		ParseMode:   telego.ModeHTML,
		ReplyMarkup: tu.InlineKeyboard(rows...),
	})

	user.LastBotMsgID = sent.GetMessageID()

}

func animateLoading(bot *telego.Bot, chatID telego.ChatID, msgID int, stop <-chan struct{}) {
	//frames := []string{"⏳ Обрабатываю.", "⏳ Обрабатываю..", "⏳ Обрабатываю...", "⏳ Обрабатываю...."}
	//frames := []string{"🔄", "🔁", "🔃", "⏳",}
	frames := []string{"🔄 Обрабатываю.", "🔁 Обрабатываю..", "🔃 Обрабатываю...", "⏳ Обрабатываю...."}
	i := 0
	for {
		select {
		case <-stop:
			return
		default:
			_, _ = bot.EditMessageText(context.Background(),
				tu.EditMessageText(chatID, msgID, frames[i%len(frames)]),
			)
			i++
			time.Sleep(500 * time.Millisecond)
		}
	}
}
