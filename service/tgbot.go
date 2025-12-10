package service

import (
	"SurfBoard/conf"
	"SurfBoard/grpcClient"
	"context"
	"log"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// --- Состояния пользователя ---
type State uint

const (
	StateDefault State = iota
	StateBenchmark
	StateXray
	StateSingBox
	StateSetupApps
	StateXrayAddDomainToFile
	StateBenchmarkAddDomainToFile
)

const (
	FileTmpRoutingBalancers = "!tmp_05-routing-balancers.json"
	FileSystemDefault       = "00-system-default.json"
	FileXwaveConf           = "xwave-conf.json"
	FileGeneratedRouting    = "05-routing-gen.json"
)

// --- Модель пользователя ---
type User struct {
	State        State
	LastBotMsgID int
	Domainlist   []string
}

var user User

// --- Запуск Telegram-бота ---
func RunTgBot(ctx context.Context, config *conf.Config, xrayClient, benchmarkClient *grpcClient.GRpcClient) {
	bot, err := telego.NewBot(config.TgBot.Token, telego.WithDiscardLogger())
	if err != nil {
		log.Fatalf("Ошибка создания бота: %v", err)
	}

	setupBotMetadata(ctx, bot)

	// Авторизация
	isUserAuthorized := func(userID int64) bool {
		for _, id := range config.TgBot.AdminIds {
			if id == userID {
				return true
			}
		}
		return false
	}

	// Получение обновлений
	updates, _ := bot.UpdatesViaLongPolling(ctx, nil)
	bh, _ := th.NewBotHandler(bot, updates)
	defer bh.Stop()

	// Регистрация всех обработчиков
	RegisterHandlers(bh, bot, config, xrayClient, benchmarkClient, isUserAuthorized)

	// Запуск обработки
	_ = bh.Start()
}

// --- Настройки описания и команд ---
func setupBotMetadata(ctx context.Context, bot *telego.Bot) {
	_ = bot.SetMyDescription(ctx, &telego.SetMyDescriptionParams{
		Description: "Приветствую! Это бот для управления VPN.\n\n" +
			"📌 Основные функции:\n" +
			"- Импорт vless:// trojan:// ss:// и добавление их в балансер X-wave\n" +
			"- Проверка VPN метод leastLoad X-wave\n" +
			"- Выбор VPN из балансера, Override balancer X-wave\n" +
			"- Инсталляция с выбором версии /обновление/удаление X-wave, S-wave, Surfboard, и других программ с github" +
			"- Скачивание/Замена файлов настроек: system-default.json, routing-balancers.json, {x-wave}config/config.json" +
			"- Быстрое добавление сайта в rules, для тестирования без сохранения в конфиге, Add routing rules" +
			"- Управлением Xwave\n" +
			"Нажмите /start для начала работы",
	})
	_ = bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{
		Commands: []telego.BotCommand{
			{Command: "start", Description: "Начать работу"},
			{Command: "help", Description: "Помощь"},
		},
	})
}

// --- Регистрация всех обработчиков ---
func RegisterHandlers(
	bh *th.BotHandler,
	bot *telego.Bot,
	config *conf.Config,
	xrayClient, benchmarkClient *grpcClient.GRpcClient,
	isUserAuthorized func(int64) bool,
) {
	registerStartHandler(bh, bot, config, isUserAuthorized)
	registerHandlers(bh, bot, config, isUserAuthorized)
	registerDeploy(bh, bot, config, isUserAuthorized)
	registerParserkHandler(bh, bot, config, xrayClient, benchmarkClient, isUserAuthorized)
	registerFilesHandler(bh, bot, config, isUserAuthorized)
	registerCallbackHandler(bh, bot, config, xrayClient, benchmarkClient, isUserAuthorized)
}
