package service

import (
	"SurfBoard/conf"
	"SurfBoard/grpcClient"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// --- Callback кнопки ---
func registerFilesHandler(
	bh *th.BotHandler,
	bot *telego.Bot,
	config *conf.Config,
	xraygRpcclient, benchmarkclient *grpcClient.GRpcClient,
	isUserAuthorized func(int64) bool,
) {
	// --- Callback "deploy_<имя>" ---
	bh.Handle(func(ctx *th.Context, update telego.Update) error {

		// === Callback от кнопок ===
		if cq := update.CallbackQuery; cq != nil {
			handleCallback(ctx, bot, cq, config)
			return nil
		}

		// === Обычные сообщения ===
		if update.Message == nil {
			return nil
		}
		msg := update.Message

		// === Если пришёл документ — сохраняем ===
		if doc := msg.Document; doc != nil {
			handleFileUpload(ctx, bot, msg.Chat.ChatID(), doc, config)
			return nil
		}

		return nil
	}, th.AnyMessageWithMedia())
}

// === Обработка callback от кнопок ===
func handleCallback(ctx context.Context, bot *telego.Bot, cq *telego.CallbackQuery, config *conf.Config) {
	chatID := cq.Message.GetChat().ChatID()
	fileName := cq.Data
	path := getTargetPath(fileName, config)

	if path == "" {
		return
	}

	bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
		Text:            fmt.Sprintf("📤 Отправляю %s...", fileName),
		ShowAlert:       false,
	})

	if _, err := os.Stat(path); os.IsNotExist(err) {
		bot.SendMessage(ctx, tu.Message(chatID, "Файл ещё не существует на сервере."))
		return
	}

	_, _ = bot.SendDocument(ctx, tu.Document(chatID, tu.File(mustOpen(path))))
}

// === Обработка загрузки нового файла ===
func handleFileUpload(ctx context.Context, bot *telego.Bot, chatID telego.ChatID, doc *telego.Document, config *conf.Config) {
	fileName := doc.FileName
	fmt.Printf("Получен файл: %s\n", fileName)

	targetPath := getTargetPath(fileName, config)
	if targetPath == "" {
		bot.SendMessage(ctx, tu.Message(chatID,
			"⚠️ Неизвестный файл. Можно заменять только:\n"+
				"- "+FileRoutingBalancers+"\n"+
				"- "+FileSystemDefault+"\n"+
				"- "+FileXwaveConf))
		return
	}

	file, err := bot.GetFile(ctx, &telego.GetFileParams{FileID: doc.FileID})
	if err != nil {
		bot.SendMessage(ctx, tu.Message(chatID, "Ошибка при получении файла"))
		return
	}

	fileURL := bot.FileDownloadURL(file.FilePath)
	resp, err := http.Get(fileURL)
	if err != nil {
		bot.SendMessage(ctx, tu.Message(chatID, "Не удалось скачать файл"))
		return
	}
	defer resp.Body.Close()

	// Создаём директорию, если её нет
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		bot.SendMessage(ctx, tu.Message(chatID, "Ошибка создания директории"))
		return
	}

	out, err := os.Create(targetPath)
	if err != nil {
		bot.SendMessage(ctx, tu.Message(chatID, "Ошибка записи файла"))
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		bot.SendMessage(ctx, tu.Message(chatID, "Ошибка при записи данных в файл"))
		return
	}

	// Отправляем подтверждение об успешной замене
	_, _ = bot.SendMessage(ctx, tu.Message(chatID, fmt.Sprintf("✅ Файл успешно заменён: %s", targetPath)))

	// И отправляем те же кнопки, чтобы можно было сразу скачать файл
	keyboard := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("📄 "+FileRoutingBalancers).WithCallbackData(FileRoutingBalancers),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("⚙️ "+FileSystemDefault).WithCallbackData(FileSystemDefault),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("🧩 "+FileXwaveConf).WithCallbackData(FileXwaveConf),
		),
	)
	_, _ = bot.SendMessage(ctx, tu.Message(chatID, "Хочешь скачать файлы? Нажми кнопку:").WithReplyMarkup(keyboard))
}

// === Возвращает путь для сохранения по имени файла ===
func getTargetPath(fileName string, config *conf.Config) string {
	template := config.XwayConf.Env.XrayLocationTemplatedir

	switch fileName {
	case FileRoutingBalancers:
		return path.Join(template, "05-routing-balancers.json")
	case FileSystemDefault:
		return path.Join(template, "00-system-default.json")
	case FileXwaveConf:
		return "/opt/etc/xwave/xwave-conf.json"
	default:
		return ""
	}
}

// === Helper для открытия файла ===
func mustOpen(filename string) *os.File {
	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	return file
}
