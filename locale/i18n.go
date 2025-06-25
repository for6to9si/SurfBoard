package locale

var messages = map[string]map[string]string{
	"en": {
		"welcome":     "Hello %s!",
		"current_vpn": "Current VPN",
		"all_vpns":    "All VPNs",
		"add_vpn":     "Add new VPN",
		"done":        "Done",
		"go_response": "GO",
	},
	"ru": {
		"welcome":     "Привет, %s!",
		"current_vpn": "Посмотреть текущий VPN",
		"all_vpns":    "Посмотреть все VPN",
		"add_vpn":     "Добавить новый VPN",
		"done":        "Готово",
		"go_response": "ПОЕХАЛИ",
	},
}

// t возвращает локализованную строку
func T(lang, key string) string {
	if msgSet, ok := messages[lang]; ok {
		if msg, ok := msgSet[key]; ok {
			return msg
		}
	}
	return key
}

// normalizeLang нормализует язык (только ru/en)
func NormalizeLang(lang string) string {
	if lang == "ru" {
		return "ru"
	}
	return "en"
}
