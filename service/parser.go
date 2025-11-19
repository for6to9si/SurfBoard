package service

import (
	"SurfBoard/benchmarkMode"
	"SurfBoard/conf"
	"SurfBoard/grpcClient"
	"SurfBoard/locale"
	"fmt"
	"net/url"
	"strconv"
	"strings"

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
		case StateBenchmark:
			text = "Thanks for your data!"
			handleVPNState(ctx, message, bot, benchmarkclient, config.BenchmarkSettings.Env.XrayLocationConfdir)

		case StateXray:
			text = "StateXray!"
			handleVPNState(ctx, message, bot, xraygRpcclient, config.XwayConf.Env.XrayLocationConfdir)

		case StateXrayAddDomainToFile:
			text = "StateXrayAddDomainToFile!"
			handleDomainState(ctx, message, bot, benchmarkclient, config.BenchmarkSettings.Env.XrayLocationConfdir)

		case StateBenchmarkAddDomainToFile:
			text = "StateBenchmarkAddDomainToFile!"
			handleDomainState(ctx, message, bot, benchmarkclient, config.BenchmarkSettings.Env.XrayLocationConfdir)
		case StateSingBox:
			text = "StateSingBox!"

		case StateSetupApps:
			text = "StateSetupApps!"

		default:
			panic("unknown state")
		}

		_, _ = bot.SendMessage(ctx, tu.Message(message.Chat.ChatID(), text))
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
	confDir string,
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

	tags := benchmarkMode.ParsesVpns(filteredLines, confDir) //обратка vless://..., ss://..

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

	// 1️⃣ Если начинается с domain: — оставить как есть
	if strings.HasPrefix(raw, "domain:") {
		return []string{raw}
	}

	// 2️⃣ Если начинается с ext: — оставить как есть
	if strings.HasPrefix(raw, "ext:") {
		return []string{raw}
	}

	// 3️⃣ Если это URL — извлечь домен
	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
		host := parsed.Host
		// убираем порт
		host = strings.Split(host, ":")[0]
		return []string{host}
	}

	// 4️⃣ Если это доменное имя (без схемы)
	if strings.Contains(raw, ".") {
		raw = strings.Split(raw, ":")[0] // убрать порт
		return []string{raw}
	}

	// 5️⃣ Иначе — игнорировать
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
	confDir string,
) {
	var all []string

	lines := strings.Split(message.Text, "\n")
	for _, line := range lines {
		all = append(all, ExtractDomainsAll(line)...)
	}

	for _, line := range all {
		if strings.TrimSpace(line) == "" {
			continue
		}
		_, _ = bot.SendMessage(ctx, tu.Message(message.Chat.ChatID(), line))
	}
}
