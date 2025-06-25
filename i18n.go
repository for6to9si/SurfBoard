package main

import (
	"embed"
	"github.com/BurntSushi/toml"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"strings"
)

//go:embed *.toml
var localeFS embed.FS

var bundle *i18n.Bundle

func initI18n() {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	files, err := localeFS.ReadDir(".")
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".toml") {
			data, err := localeFS.ReadFile(file.Name())
			if err != nil {
				panic(err)
			}
			_, err = bundle.ParseMessageFileBytes(data, file.Name())
			if err != nil {
				panic(err)
			}
		}
	}
}

func localizer(lang string) *i18n.Localizer {
	tag := language.English
	if lang == "ru" {
		tag = language.Russian
	}
	return i18n.NewLocalizer(bundle, tag.String())
}
