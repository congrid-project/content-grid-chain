package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"
)

//go:embed templates/*.html templates/partials/*.html static/* static/assets/*
var siteFS embed.FS

type server struct {
	templates map[string]*template.Template
	static    http.Handler
	slotStore SlotStore
	walletCfg WalletConfig
	regCfg    PublisherRegisterConfig
}

func main() {
	var (
		addr       = flag.String("addr", ":8080", "listen address")
		baseURL    = flag.String("base-url", "https://congrid.net", "public base URL used in copyable snippets")
		requestLog = flag.Bool("request-log", true, "log requests")
		downloads  = flag.String("downloads-dir", defaultDownloadsDir(), "directory containing public release downloads")

		airdropEnabled = flag.Bool("airdrop", false, "enable airdrop endpoint (requires funded faucet key)")
		airdropDB      = flag.String("airdrop-db", "./congrid-airdrop.db", "path to airdrop claim database")
		chainID        = flag.String("chain-id", "", "chain id")
		nodeRPC        = flag.String("node", "", "rpc endpoint (e.g. tcp://127.0.0.1:26657)")
		walletRPC      = flag.String("wallet-rpc", "", "public rpc endpoint used by browser wallets (defaults to base-url /rpc reverse proxy)")
		walletREST     = flag.String("wallet-rest", "", "public rest endpoint used by browser wallets (defaults to base-url /rest reverse proxy)")
		denom          = flag.String("denom", "ucongrid", "fee token denom")
		amount         = flag.String("airdrop-amount", "25000", "amount (in denom base units) to send per domain")
		faucetKeyName  = flag.String("faucet-key", "faucet", "local keyring key name used by content-grid-d")
		contentGridBin = flag.String("content-grid-bin", defaultContentGridBin(), "content-grid-d executable path/name for server-side tx helpers (or CONTENT_GRID_BIN)")
		keyringBackend = flag.String("keyring-backend", "test", "keyring backend for content-grid-d")
		keyringDir     = flag.String("keyring-dir", "", "optional keyring directory for content-grid-d")
		fees           = flag.String("fees", "", "optional explicit fees (e.g. 0ucongrid or 2000stake)")
		gasPrices      = flag.String("gas-prices", "", "optional gas prices (e.g. 0.001ucongrid)")

		slotsStore       = flag.String("slots-store", "chain", "slot store backend (chain)")
		slotsGRPC        = flag.String("slots-grpc", "", "grpc endpoint for chain slot queries (e.g. localhost:9090)")
		slotsRateDenom   = flag.String("slot-rate-denom", "ucongrid", "slot rate denom")
		slotsUnitSeconds = flag.Int64("slot-unit-seconds", 7*24*60*60, "slot billing unit in seconds")
		slotsMinDuration = flag.Int64("slot-min-duration-seconds", 7*24*60*60, "minimum slot lease duration in seconds")
		slotsMaxDuration = flag.Int64("slot-max-duration-seconds", 90*24*60*60, "maximum slot lease duration in seconds")
	)
	flag.Parse()

	templates, err := buildPageTemplates(siteFS)
	if err != nil {
		log.Fatalf("template init: %v", err)
	}

	subStatic := mustSub(siteFS, "static")
	// http.FileServer expects an fs.FS wrapped with http.FS.
	if !strings.EqualFold(strings.TrimSpace(*slotsStore), "chain") {
		log.Fatalf("slots-store must be chain")
	}
	if strings.TrimSpace(*slotsGRPC) == "" {
		log.Fatalf("slots-grpc required for chain-backed marketplace")
	}
	chainStore, err := newChainSlotStore(chainSlotConfig{
		GRPCAddr: *slotsGRPC,
	})
	if err != nil {
		log.Fatalf("slot store init: %v", err)
	}
	var slotStore SlotStore = chainStore
	var slotCloser interface{ Close() error } = chainStore

	publicWalletRPC, publicWalletREST, err := resolveWalletEndpoints(*baseURL, *nodeRPC, *walletRPC, *walletREST)
	if err != nil {
		log.Fatalf("wallet endpoint resolve: %v", err)
	}
	publicWalletRPCDefault := deriveWalletProxyEndpoint(*baseURL, walletRPCProxyPath)
	publicWalletRESTDefault := deriveWalletProxyEndpoint(*baseURL, walletRESTProxyPath)

	var walletRPCProxy http.Handler
	if publicWalletRPC == publicWalletRPCDefault {
		walletRPCProxy, err = newWalletRPCProxy(*nodeRPC)
		if err != nil {
			log.Fatalf("wallet rpc proxy init: %v", err)
		}
	}

	var walletRESTProxy http.Handler
	if publicWalletREST == publicWalletRESTDefault {
		walletRESTProxy, err = newWalletRESTProxy(*nodeRPC)
		if err != nil {
			log.Fatalf("wallet rest proxy init: %v", err)
		}
	}

	walletCfg := WalletConfig{
		Enabled:                true,
		ChainID:                strings.TrimSpace(*chainID),
		RPC:                    publicWalletRPC,
		REST:                   publicWalletREST,
		FeeDenom:               strings.TrimSpace(*denom),
		GasPrice:               strings.TrimSpace(*gasPrices),
		SlotRateDenom:          strings.TrimSpace(*slotsRateDenom),
		SlotUnitSeconds:        *slotsUnitSeconds,
		SlotMinDurationSeconds: *slotsMinDuration,
		SlotMaxDurationSeconds: *slotsMaxDuration,
		GasCreateSlot:          220000,
		GasUpdateSlot:          140000,
		GasLeaseSlot:           220000,
	}
	if walletCfg.GasPrice == "" {
		walletCfg.GasPrice = "0.001ucongrid"
	}
	if walletCfg.ChainID == "" || walletCfg.RPC == "" || walletCfg.REST == "" {
		log.Fatalf("chain-id, wallet rpc, and wallet rest are required for wallet signing")
	}

	regCfg := PublisherRegisterConfig{
		ChainID:        strings.TrimSpace(*chainID),
		NodeRPC:        strings.TrimSpace(*nodeRPC),
		ContentGridBin: strings.TrimSpace(*contentGridBin),
		KeyringBackend: strings.TrimSpace(*keyringBackend),
		KeyringDir:     strings.TrimSpace(*keyringDir),
		Fees:           strings.TrimSpace(*fees),
		GasPrices:      strings.TrimSpace(*gasPrices),
	}

	s := &server{
		templates: templates,
		static:    http.FileServer(http.FS(subStatic)),
		slotStore: slotStore,
		walletCfg: walletCfg,
		regCfg:    regCfg,
	}
	downloadHandler, err := newDownloadHandler(*downloads)
	if err != nil {
		log.Fatalf("downloads init: %v", err)
	}
	log.Printf("congrid-site downloads served from %s", downloadHandler.root)

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))
	mux.Handle("GET /downloads/{filename}", downloadHandler)
	if walletRPCProxy != nil {
		mux.Handle(walletRPCProxyPath, walletRPCProxy)
		mux.Handle(walletRPCProxyPath+"/", walletRPCProxy)
	}
	if walletRESTProxy != nil {
		mux.Handle(walletRESTProxyPath, walletRESTProxy)
		mux.Handle(walletRESTProxyPath+"/", walletRESTProxy)
	}
	mux.HandleFunc("GET /{$}", s.handleHome(*baseURL))
	mux.HandleFunc("GET /publishers", s.handlePublishers(*baseURL))
	mux.HandleFunc("POST /publishers/register", s.handlePublisherRegister(*baseURL))
	mux.HandleFunc("GET /verifiers", s.handleVerifiers(*baseURL))
	mux.HandleFunc("GET /docs", s.handleDocs(*baseURL))
	mux.HandleFunc("GET /marketplace", s.handleMarketplace(*baseURL))
	mux.HandleFunc("GET /leases", s.handleLeases(*baseURL))
	mux.HandleFunc("POST /marketplace/lease", s.handleMarketplaceLease(*baseURL))
	mux.HandleFunc("GET /publisher/dashboard", s.handlePublisherDashboard(*baseURL))
	mux.HandleFunc("POST /publisher/dashboard", s.handlePublisherDashboardPost(*baseURL))
	mux.HandleFunc("GET /badge.png", s.handleBadgePNG)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	if *airdropEnabled {
		cfg := airdropConfig{
			DBPath:         *airdropDB,
			ChainID:        *chainID,
			NodeRPC:        *nodeRPC,
			Denom:          *denom,
			Amount:         *amount,
			FaucetKeyName:  *faucetKeyName,
			ContentGridBin: strings.TrimSpace(*contentGridBin),
			Keyring:        *keyringBackend,
			KeyringDir:     strings.TrimSpace(*keyringDir),
			Fees:           *fees,
			GasPrices:      *gasPrices,
			BaseURL:        *baseURL,
		}
		air, err := newAirdropper(s, cfg)
		if err != nil {
			log.Fatalf("airdrop init: %v", err)
		}
		mux.HandleFunc("GET /airdrop", air.handleAirdropGet())
		mux.HandleFunc("POST /airdrop", air.handleAirdropPost())
	} else {
		mux.HandleFunc("GET /airdrop", s.handleAirdropUnavailable(*baseURL))
		mux.HandleFunc("POST /airdrop", s.handleAirdropUnavailablePost(*baseURL))
	}

	h := http.Handler(mux)
	if *requestLog {
		h = withRequestLog(h)
	}

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("congrid-site listening on %s", *addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop

	if slotCloser != nil {
		_ = slotCloser.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func buildPageTemplates(efs embed.FS) (map[string]*template.Template, error) {
	funcs := template.FuncMap{
		"toJSON": func(v any) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("{}")
			}
			return template.JS(b)
		},
		"nowYear": func() int { return time.Now().Year() },
		"formatDate": func(t time.Time) string {
			return formatDate(t)
		},
		"formatRate": func(rate int) string {
			return formatRate(rate)
		},
		"formatTraffic": func(traffic int) string {
			return formatTraffic(traffic)
		},
		"slotStatusLabel": func(status SlotStatus) string {
			return slotStatusLabel(status)
		},
		"slotStatusClass": func(status SlotStatus) string {
			return slotStatusClass(status)
		},
		"slotAvailabilityLabel": func(slot Slot) string {
			return slotAvailabilityLabel(slot)
		},
		"slotAvailabilityClass": func(slot Slot) string {
			return slotAvailabilityClass(slot)
		},
		"slotIsAvailable": func(slot Slot) bool {
			return slotIsAvailable(slot)
		},
		"slotDurationOptions": func(slot Slot) []DurationOption {
			return slotDurationOptions(slot)
		},
		"leaseStatusLabel": func(lease SlotLease) string {
			return leaseStatusLabel(lease)
		},
		"leaseStatusClass": func(lease SlotLease) string {
			return leaseStatusClass(lease)
		},
		"formatLeaseTerm": func(lease SlotLease) string {
			return formatLeaseTerm(lease)
		},
		"leaseEmbedSnippet": func(lease SlotLease) string {
			return leaseEmbedSnippet(lease)
		},
		"trimScheme": func(u string) string {
			u = strings.TrimSpace(u)
			u = strings.TrimPrefix(u, "https://")
			u = strings.TrimPrefix(u, "http://")
			return strings.TrimRight(u, "/")
		},
	}

	entries, err := fs.ReadDir(efs, "templates")
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}

	out := make(map[string]*template.Template)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		t, err := template.New("").Funcs(funcs).ParseFS(efs,
			"templates/partials/*.html",
			path.Join("templates", name),
		)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		out[name] = t
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no page templates found")
	}
	return out, nil
}

func (s *server) render(w http.ResponseWriter, name string, data any) {
	t, ok := s.templates[name]
	if !ok {
		log.Printf("render %s: template not found", name)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
}

type pageData struct {
	Title        string
	Description  string
	BaseURL      string
	Path         string
	NowYear      int
	Flash        string
	WalletConfig WalletConfig
}

type PublisherRegisterConfig struct {
	ChainID        string
	NodeRPC        string
	ContentGridBin string
	KeyringBackend string
	KeyringDir     string
	Fees           string
	GasPrices      string
}

type WalletConfig struct {
	Enabled                bool   `json:"enabled"`
	ChainID                string `json:"chain_id"`
	RPC                    string `json:"rpc"`
	REST                   string `json:"rest"`
	FeeDenom               string `json:"fee_denom"`
	GasPrice               string `json:"gas_price"`
	SlotRateDenom          string `json:"slot_rate_denom"`
	SlotUnitSeconds        int64  `json:"slot_unit_seconds"`
	SlotMinDurationSeconds int64  `json:"slot_min_duration_seconds"`
	SlotMaxDurationSeconds int64  `json:"slot_max_duration_seconds"`
	GasCreateSlot          int64  `json:"gas_create_slot"`
	GasUpdateSlot          int64  `json:"gas_update_slot"`
	GasLeaseSlot           int64  `json:"gas_lease_slot"`
}

func resolveWalletEndpoints(baseURL, nodeRPC, walletRPC, walletREST string) (string, string, error) {
	defaultScheme := walletEndpointDefaultScheme(baseURL)

	rpc := strings.TrimSpace(walletRPC)
	if rpc != "" {
		rpc = normalizeWalletEndpoint(rpc, defaultScheme)
	} else {
		rpc = deriveWalletRPCEndpoint(baseURL, nodeRPC)
	}
	if rpc == "" {
		return "", "", fmt.Errorf("wallet rpc endpoint required")
	}

	rest := strings.TrimSpace(walletREST)
	if rest != "" {
		rest = normalizeWalletEndpoint(rest, defaultScheme)
	} else {
		rest = deriveWalletRESTEndpoint(baseURL, rpc)
	}
	if rest == "" {
		return "", "", fmt.Errorf("wallet rest endpoint required")
	}

	return rpc, rest, nil
}

func deriveWalletRPCEndpoint(baseURL, nodeRPC string) string {
	if strings.TrimSpace(nodeRPC) == "" {
		return ""
	}
	return deriveWalletProxyEndpoint(baseURL, walletRPCProxyPath)
}

func deriveWalletRESTEndpoint(baseURL, walletRPC string) string {
	if isWalletProxyEndpoint(baseURL, walletRPC, walletRPCProxyPath) {
		return deriveWalletProxyEndpoint(baseURL, walletRESTProxyPath)
	}

	defaultScheme := walletEndpointDefaultScheme(baseURL)
	parsed, err := url.Parse(normalizeWalletEndpoint(walletRPC, defaultScheme))
	if err != nil || parsed.Host == "" {
		return ""
	}

	if parsed.Path != "" && parsed.Path != "/" {
		siblingPath, ok := deriveWalletRESTSiblingPath(parsed.Path)
		if !ok {
			return ""
		}
		parsed.Path = siblingPath
		parsed.RawPath = ""
		return parsed.String()
	}

	setURLPort(parsed, "1317")
	return parsed.String()
}

func walletEndpointDefaultScheme(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err == nil {
		switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
		case "https":
			return "https"
		case "http":
			return "http"
		}
	}
	return "http"
}

func normalizeWalletEndpoint(raw, defaultScheme string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if defaultScheme == "" {
		defaultScheme = "http"
	}
	if strings.HasPrefix(raw, "tcp://") {
		raw = defaultScheme + "://" + strings.TrimPrefix(raw, "tcp://")
	}
	if !strings.Contains(raw, "://") {
		raw = defaultScheme + "://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme == "" {
		parsed.Scheme = defaultScheme
	}
	return parsed.String()
}

func setURLPort(u *url.URL, port string) {
	if u == nil {
		return
	}
	host := u.Hostname()
	if host == "" {
		return
	}
	if port == "" {
		u.Host = host
		return
	}
	u.Host = net.JoinHostPort(host, port)
}

func (s *server) handleHome(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, "home.html", pageData{
			Title:        "Congrid — Content Grid Protocol",
			Description:  "A decentralized similar-site interconnection protocol where publishers get free backlinks and ongoing Congrid token rewards, while verifiers earn more by validating publisher status.",
			BaseURL:      baseURL,
			Path:         r.URL.Path,
			NowYear:      time.Now().Year(),
			WalletConfig: s.walletCfg,
		})
	}
}

func (s *server) handlePublishers(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, "publishers.html", pageData{
			Title:        "Become a Publisher — Congrid",
			Description:  "Register your site, add the Congrid verification badge, and earn rewards while sending high-quality referral traffic across the open web.",
			BaseURL:      baseURL,
			Path:         r.URL.Path,
			NowYear:      time.Now().Year(),
			WalletConfig: s.walletCfg,
		})
	}
}

func (s *server) handleVerifiers(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, "verifiers.html", pageData{
			Title:        "Become a Verifier — Congrid",
			Description:  "Run verifier software to help confirm publishers and earn a share of the network’s rewards.",
			BaseURL:      baseURL,
			Path:         r.URL.Path,
			NowYear:      time.Now().Year(),
			WalletConfig: s.walletCfg,
		})
	}
}

func (s *server) handleDocs(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, "docs.html", pageData{
			Title:        "Guides — Congrid",
			Description:  "Publisher and verifier guides for joining Congrid, including badge setup, registration, bonding, and verifier workflow.",
			BaseURL:      baseURL,
			Path:         r.URL.Path,
			NowYear:      time.Now().Year(),
			WalletConfig: s.walletCfg,
		})
	}
}

func (s *server) handleBadgePNG(w http.ResponseWriter, r *http.Request) {
	publisher := strings.TrimSpace(r.URL.Query().Get("publisher"))
	wallet := strings.TrimSpace(r.URL.Query().Get("wallet"))

	// A small, deterministic badge that can be embedded by publishers.
	// The query params are not required to render, but are preserved for future attribution/analytics.
	const (
		wPx = 320
		hPx = 84
	)

	img := image.NewRGBA(image.Rect(0, 0, wPx, hPx))
	bg := color.RGBA{R: 18, G: 24, B: 38, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	// Border
	border := color.RGBA{R: 96, G: 165, B: 250, A: 255}
	for x := 0; x < wPx; x++ {
		img.Set(x, 0, border)
		img.Set(x, hPx-1, border)
	}
	for y := 0; y < hPx; y++ {
		img.Set(0, y, border)
		img.Set(wPx-1, y, border)
	}

	// Render simple text using a tiny built-in raster font.
	// This keeps the binary self-contained with no external font deps.
	lines := []string{"Verified by Congrid"}
	if publisher != "" {
		lines = append(lines, "publisher: "+publisher)
	}
	if wallet != "" {
		lines = append(lines, "wallet: "+shorten(wallet, 18))
	}

	writeText(img, 14, 18, lines[0], color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if len(lines) > 1 {
		writeText(img, 14, 42, lines[1], color.RGBA{R: 203, G: 213, B: 225, A: 255})
	}
	if len(lines) > 2 {
		writeText(img, 14, 62, lines[2], color.RGBA{R: 148, G: 163, B: 184, A: 255})
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_ = png.Encode(w, img)
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s (%s)", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

func mustSub(efs embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(efs, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 8 {
		return s[:n]
	}
	return s[:n-5] + "…" + s[len(s)-4:]
}

// ---- Tiny bitmap font renderer (uppercase/lowercase/digits/basic punctuation). ----

var font = map[rune][7]byte{
	' ': {0, 0, 0, 0, 0, 0, 0},
	':': {0, 8, 8, 0, 8, 8, 0},
	'-': {0, 0, 0, 31, 0, 0, 0},
	'.': {0, 0, 0, 0, 0, 12, 12},
	'/': {1, 2, 4, 8, 16, 0, 0},
	'_': {0, 0, 0, 0, 0, 0, 31},
	'…': {0, 0, 0, 0, 21, 0, 0},
	'a': {0, 14, 1, 15, 17, 15, 0},
	'b': {16, 16, 30, 17, 17, 30, 0},
	'c': {0, 14, 16, 16, 16, 14, 0},
	'd': {1, 1, 15, 17, 17, 15, 0},
	'e': {0, 14, 17, 31, 16, 14, 0},
	'f': {6, 8, 30, 8, 8, 8, 0},
	'g': {0, 15, 17, 15, 1, 14, 0},
	'h': {16, 16, 30, 17, 17, 17, 0},
	'i': {4, 0, 12, 4, 4, 14, 0},
	'j': {2, 0, 6, 2, 18, 12, 0},
	'k': {16, 18, 20, 24, 20, 18, 0},
	'l': {12, 4, 4, 4, 4, 14, 0},
	'm': {0, 26, 21, 21, 21, 21, 0},
	'n': {0, 30, 17, 17, 17, 17, 0},
	'o': {0, 14, 17, 17, 17, 14, 0},
	'p': {0, 30, 17, 30, 16, 16, 0},
	'q': {0, 15, 17, 15, 1, 1, 0},
	'r': {0, 22, 24, 16, 16, 16, 0},
	's': {0, 15, 16, 14, 1, 30, 0},
	't': {8, 8, 30, 8, 8, 6, 0},
	'u': {0, 17, 17, 17, 19, 13, 0},
	'v': {0, 17, 17, 17, 10, 4, 0},
	'w': {0, 17, 17, 21, 21, 10, 0},
	'x': {0, 17, 10, 4, 10, 17, 0},
	'y': {0, 17, 17, 15, 1, 14, 0},
	'z': {0, 31, 2, 4, 8, 31, 0},
	'A': {14, 17, 17, 31, 17, 17, 0},
	'B': {30, 17, 30, 17, 17, 30, 0},
	'C': {15, 16, 16, 16, 16, 15, 0},
	'D': {30, 17, 17, 17, 17, 30, 0},
	'E': {31, 16, 30, 16, 16, 31, 0},
	'F': {31, 16, 30, 16, 16, 16, 0},
	'G': {15, 16, 16, 19, 17, 15, 0},
	'H': {17, 17, 31, 17, 17, 17, 0},
	'I': {14, 4, 4, 4, 4, 14, 0},
	'J': {7, 2, 2, 2, 18, 12, 0},
	'K': {17, 18, 28, 18, 17, 17, 0},
	'L': {16, 16, 16, 16, 16, 31, 0},
	'M': {17, 27, 21, 17, 17, 17, 0},
	'N': {17, 25, 21, 19, 17, 17, 0},
	'O': {14, 17, 17, 17, 17, 14, 0},
	'P': {30, 17, 30, 16, 16, 16, 0},
	'Q': {14, 17, 17, 17, 19, 15, 0},
	'R': {30, 17, 30, 18, 17, 17, 0},
	'S': {15, 16, 14, 1, 1, 30, 0},
	'T': {31, 4, 4, 4, 4, 4, 0},
	'U': {17, 17, 17, 17, 17, 14, 0},
	'V': {17, 17, 17, 17, 10, 4, 0},
	'W': {17, 17, 17, 21, 21, 10, 0},
	'X': {17, 10, 4, 4, 10, 17, 0},
	'Y': {17, 10, 4, 4, 4, 4, 0},
	'Z': {31, 2, 4, 8, 16, 31, 0},
	'0': {14, 17, 19, 21, 25, 14, 0},
	'1': {4, 12, 4, 4, 4, 14, 0},
	'2': {14, 17, 2, 4, 8, 31, 0},
	'3': {31, 2, 4, 2, 17, 14, 0},
	'4': {2, 6, 10, 18, 31, 2, 0},
	'5': {31, 16, 30, 1, 17, 14, 0},
	'6': {6, 8, 30, 17, 17, 14, 0},
	'7': {31, 1, 2, 4, 8, 8, 0},
	'8': {14, 17, 14, 17, 17, 14, 0},
	'9': {14, 17, 17, 15, 1, 6, 0},
}

func writeText(img *image.RGBA, x, y int, s string, c color.Color) {
	x0 := x
	for _, r := range s {
		if r == '\n' {
			y += 10
			x = x0
			continue
		}
		glyph, ok := font[r]
		if !ok {
			glyph = font['?']
		}
		drawGlyph(img, x, y, glyph, c)
		x += 6
	}
}

func drawGlyph(img *image.RGBA, x, y int, g [7]byte, c color.Color) {
	for row := 0; row < 7; row++ {
		bits := g[row]
		for col := 0; col < 5; col++ {
			if bits&(1<<(4-col)) != 0 {
				img.Set(x+col, y+row, c)
			}
		}
	}
}

func init() {
	// Add a fallback '?' glyph.
	if _, ok := font['?']; !ok {
		font['?'] = [7]byte{14, 17, 2, 4, 0, 4, 0}
	}
}

// Ensure unused imports remain pruned by referencing path package where needed.
var _ = fmt.Sprintf
var _ = path.Clean
