package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEveryPageRendersAllLanguages(t *testing.T) {
	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)
	s := &server{
		templates: templates,
		slotStore: newMemorySlotStore(),
		walletCfg: WalletConfig{Enabled: true, ChainID: "congrid-main", RPC: "https://congrid.net/rpc"},
	}
	air := &airdropper{srv: s, cfg: airdropConfig{BaseURL: "https://congrid.net"}}
	pages := []struct {
		path    string
		handler http.HandlerFunc
		zh, fr  string
	}{
		{"/", s.handleHome("https://congrid.net"), "让其他 Congrid 成员网站的读者发现您", "Faites-vous découvrir sur les autres sites membres de Congrid"},
		{"/publishers", s.handlePublishers("https://congrid.net"), "第 3 步 — 链上注册", "Étape 3 — S’inscrire sur chaîne"},
		{"/verifiers", s.handleVerifiers("https://congrid.net"), "第 1 步 — 安装原生验证者服务", "Étape 1 — Installer les services natifs de vérification"},
		{"/docs", s.handleDocs("https://congrid.net"), "发布者与验证者指南", "Guides des éditeurs et des vérificateurs"},
		{"/marketplace", s.handleMarketplace("https://congrid.net"), "市场（已弃用）", "Marché (obsolète)"},
		{"/leases", s.handleLeases("https://congrid.net"), "租约发布面板", "Tableau de publication des locations"},
		{"/publisher/dashboard", s.handlePublisherDashboard("https://congrid.net"), "发布者控制台", "Tableau de bord de l’éditeur"},
		{"/airdrop", s.handleAirdropUnavailable("https://congrid.net"), "目前尚未开放空投领取", "Les demandes d’airdrop ne sont pas activées pour le moment"},
		{"/airdrop", air.handleAirdropGet(), "领取可选启动空投", "Demander l’airdrop de démarrage facultatif"},
	}
	for index, page := range pages {
		for _, lang := range []string{"en", "zh", "fr"} {
			t.Run(strconv.Itoa(index)+"/"+lang, func(t *testing.T) {
				t.Parallel()
				r := httptest.NewRequest(http.MethodGet, page.path+"?lang="+lang, nil)
				w := httptest.NewRecorder()
				page.handler(w, r)
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, lang, w.Header().Get("Content-Language"))
				body := w.Body.String()
				require.Contains(t, body, `<html lang="`+lang+`">`)
				require.Contains(t, body, `class="language-switch"`)
				require.Contains(t, body, `hreflang="`+lang+`" aria-current="true"`)
				if page.path == "/" {
					require.Contains(t, body, `src="/static/assets/congrid-architecture.png"`)
					require.NotContains(t, body, `class="architecture"`)
				}
				if lang == "zh" {
					require.Contains(t, body, page.zh)
				} else if lang == "fr" {
					require.Contains(t, body, page.fr)
				}
			})
		}
	}
}

func TestLanguageSelectionPersistsAcrossPagesAndForms(t *testing.T) {
	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)
	s := &server{templates: templates}
	w := httptest.NewRecorder()
	s.handleHome("https://congrid.net")(w, httptest.NewRequest(http.MethodGet, "/?lang=fr", nil))
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, languageCookie, cookies[0].Name)
	require.Equal(t, "fr", cookies[0].Value)
	require.Equal(t, "/", cookies[0].Path)

	r := httptest.NewRequest(http.MethodGet, "/publishers", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	s.handlePublishers("https://congrid.net")(w, r)
	require.Contains(t, w.Body.String(), `<html lang="fr">`)

	r = httptest.NewRequest(http.MethodPost, "/publishers/register", strings.NewReader("domain=invalid"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	s.handlePublisherRegister("https://congrid.net")(w, r)
	require.Contains(t, w.Body.String(), `<html lang="fr">`)
	require.Contains(t, w.Body.String(), "Format de domaine invalide.")

	r = httptest.NewRequest(http.MethodGet, "/docs?lang=zh", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	s.handleDocs("https://congrid.net")(w, r)
	require.Equal(t, "zh", w.Header().Get("Content-Language"))
	require.Equal(t, "zh", w.Result().Cookies()[0].Value)
}

func TestLanguageFallbackAndSwitchPreserveQuery(t *testing.T) {
	for _, tc := range []struct{ query, cookie, want string }{
		{"", "", "en"}, {"zh", "fr", "zh"}, {"fr", "zh", "fr"},
		{"en", "zh", "en"}, {"invalid", "fr", "fr"}, {"invalid", "invalid", "en"},
	} {
		r := httptest.NewRequest(http.MethodGet, "/docs?lang="+tc.query, nil)
		r.AddCookie(&http.Cookie{Name: languageCookie, Value: tc.cookie})
		require.Equal(t, tc.want, requestLanguage(r))
	}
	r := httptest.NewRequest(http.MethodGet, "/marketplace?q=science%20%26%20news&sort=price-low&page=2&lang=en", nil)
	u, err := url.Parse(languageURL(r, "fr"))
	require.NoError(t, err)
	require.Equal(t, "/marketplace", u.Path)
	require.Equal(t, "science & news", u.Query().Get("q"))
	require.Equal(t, "price-low", u.Query().Get("sort"))
	require.Equal(t, "2", u.Query().Get("page"))
	require.Equal(t, "fr", u.Query().Get("lang"))
}

func TestTranslatedNoticesPreserveValuesAndEscapeHTML(t *testing.T) {
	require.Equal(t, "此网站已领取一次性空投。交易：ABC123", translate("zh", "This website has already claimed its one-time airdrop. Tx: ABC123"))
	require.Equal(t, "Site vérifié. L’airdrop de 25000ucongrid est en attente pour congrid1owner.", translate("fr", "Website verified. The 25000ucongrid airdrop is queued for congrid1owner."))
	require.Equal(t, "1.2 百万 / 月", translate("zh", "1.2M / mo"))
	require.Equal(t, "2 semaines", translate("fr", "2 weeks"))
	require.Equal(t, "已连接：{1}", translate("zh", "Connected: {0}", "{1}", "must not replace user content"))

	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/publishers?lang=zh", nil)
	(&server{templates: templates}).render(w, r, "publishers.html", pageData{
		Flash: "Registration tx failed: <script>alert(1)</script>",
	})
	require.Contains(t, w.Body.String(), "注册交易失败：&lt;script&gt;alert(1)&lt;/script&gt;")
	require.NotContains(t, w.Body.String(), "<script>alert(1)</script>")
}

func TestTemplateTranslationCoverage(t *testing.T) {
	keyPattern := regexp.MustCompile(`\{\{t ("(?:[^"\\]|\\.)*")`)
	err := fs.WalkDir(siteFS, "templates", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		data, err := siteFS.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range keyPattern.FindAllSubmatch(data, -1) {
			var key string
			require.NoError(t, json.Unmarshal(match[1], &key))
			entry, ok := translations[key]
			require.True(t, ok, "%s is missing translation for %q", path, key)
			require.NotEmpty(t, entry.ZH)
			require.NotEmpty(t, entry.FR)
		}
		return nil
	})
	require.NoError(t, err)
}
