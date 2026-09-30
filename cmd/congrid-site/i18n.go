package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const languageCookie = "congrid_language"

type translationEntry struct {
	ZH     string `json:"zh"`
	FR     string `json:"fr"`
	Client bool   `json:"client,omitempty"`
}

type messagePattern struct {
	key string
	re  *regexp.Regexp
}

var translations, messagePatterns = loadTranslations()

var messagePlaceholder = regexp.MustCompile(`\{(\d+)\}`)

func loadTranslations() (map[string]translationEntry, []messagePattern) {
	data, err := siteFS.ReadFile("static/translations.json")
	if err != nil {
		panic(fmt.Sprintf("read translations: %v", err))
	}
	var catalog map[string]translationEntry
	if err := json.Unmarshal(data, &catalog); err != nil {
		panic(fmt.Sprintf("parse translations: %v", err))
	}
	var patterns []messagePattern
	for key, entry := range catalog {
		if entry.ZH == "" || entry.FR == "" {
			panic(fmt.Sprintf("incomplete translation: %q", key))
		}
		if strings.Contains(key, "{0}") {
			expr := regexp.QuoteMeta(key)
			for i := 0; strings.Contains(key, fmt.Sprintf("{%d}", i)); i++ {
				expr = strings.ReplaceAll(expr, regexp.QuoteMeta(fmt.Sprintf("{%d}", i)), "(.+?)")
			}
			patterns = append(patterns, messagePattern{key: key, re: regexp.MustCompile("^" + expr + "$")})
		}
	}
	// More specific patterns (e.g. millions/month) precede generic ones.
	sort.Slice(patterns, func(i, j int) bool {
		if len(patterns[i].key) == len(patterns[j].key) {
			return patterns[i].key < patterns[j].key
		}
		return len(patterns[i].key) > len(patterns[j].key)
	})
	return catalog, patterns
}

func validLanguage(lang string) bool {
	return lang == "en" || lang == "zh" || lang == "fr"
}

func requestLanguage(r *http.Request) string {
	if lang := r.URL.Query().Get("lang"); validLanguage(lang) {
		return lang
	}
	if cookie, err := r.Cookie(languageCookie); err == nil && validLanguage(cookie.Value) {
		return cookie.Value
	}
	return "en"
}

// translate also handles formatted notices without translating addresses, IDs,
// hashes, or other values supplied by the chain or the user.
func translate(lang, message string, args ...any) string {
	translated := message
	if entry, ok := translations[message]; ok {
		switch lang {
		case "zh":
			translated = entry.ZH
		case "fr":
			translated = entry.FR
		}
	} else if lang != "en" {
		for _, pattern := range messagePatterns {
			if matches := pattern.re.FindStringSubmatch(message); matches != nil {
				values := make([]any, len(matches)-1)
				for i, value := range matches[1:] {
					values[i] = value
				}
				return translate(lang, pattern.key, values...)
			}
		}
	}
	return messagePlaceholder.ReplaceAllStringFunc(translated, func(placeholder string) string {
		index, err := strconv.Atoi(placeholder[1 : len(placeholder)-1])
		if err != nil || index >= len(args) {
			return placeholder
		}
		return fmt.Sprint(args[index])
	})
}

func languageURL(r *http.Request, lang string) string {
	u := *r.URL
	query := u.Query()
	query.Set("lang", lang)
	u.RawQuery = query.Encode()
	return u.RequestURI()
}

func languageTemplateFuncs(lang string, r *http.Request) template.FuncMap {
	return template.FuncMap{
		"t":        func(message string, args ...any) string { return translate(lang, message, args...) },
		"language": func() string { return lang },
		"languageURL": func(target string) string {
			if r == nil {
				return "?lang=" + target
			}
			return languageURL(r, target)
		},
		"clientMessages": func() map[string]string {
			messages := make(map[string]string)
			for key, entry := range translations {
				if entry.Client {
					messages[key] = translate(lang, key)
				}
			}
			return messages
		},
	}
}
