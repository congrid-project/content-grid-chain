package main

import (
	"context"
	"embed"
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
	"net/http"
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
}

func main() {
	var (
		addr       = flag.String("addr", ":8080", "listen address")
		baseURL    = flag.String("base-url", "https://congrid.net", "public base URL used in copyable snippets")
		requestLog = flag.Bool("request-log", true, "log requests")

		airdropEnabled = flag.Bool("airdrop", false, "enable airdrop endpoint (requires funded faucet key)")
		airdropDB      = flag.String("airdrop-db", "./congrid-airdrop.db", "path to airdrop claim database")
		chainID        = flag.String("chain-id", "", "chain id")
		nodeRPC        = flag.String("node", "", "rpc endpoint (e.g. tcp://127.0.0.1:26657)")
		denom          = flag.String("denom", "ucongrid", "fee token denom")
		amount         = flag.String("airdrop-amount", "25000", "amount (in denom base units) to send per domain")
		faucetKeyName  = flag.String("faucet-key", "faucet", "local keyring key name used by content-grid-d")
		keyringBackend = flag.String("keyring-backend", "test", "keyring backend for content-grid-d")
		keyringDir     = flag.String("keyring-dir", "", "optional keyring directory for content-grid-d")
		fees           = flag.String("fees", "", "optional explicit fees (e.g. 0ucongrid or 2000stake)")
		gasPrices      = flag.String("gas-prices", "", "optional gas prices (e.g. 0.001ucongrid)")

		slotsStore         = flag.String("slots-store", "memory", "slot store backend (memory|chain)")
		slotsGRPC          = flag.String("slots-grpc", "", "grpc endpoint for chain slot queries (e.g. localhost:9090)")
		slotsKeyName       = flag.String("slots-key", "", "keyring key name for slot txs")
		leaseKeyName       = flag.String("lease-key", "", "keyring key name for lease txs (defaults to slots-key)")
		slotsHome          = flag.String("slots-home", "", "content-grid-d home directory for slot/lease txs (optional)")
		slotsBinary        = flag.String("slots-binary", "./content-grid-d", "content-grid-d binary path for slot txs")
		slotsFees          = flag.String("slot-fees", "", "optional explicit fees for slot txs")
		slotsGasPrices     = flag.String("slot-gas-prices", "", "optional gas prices for slot txs")
		slotsGas           = flag.String("slot-gas", "auto", "gas limit for slot txs")
		slotsGasAdjustment = flag.String("slot-gas-adjustment", "1.3", "gas adjustment for slot txs")
		slotsRateDenom     = flag.String("slot-rate-denom", "ucongrid", "slot rate denom")
		slotsUnitSeconds   = flag.Int64("slot-unit-seconds", 7*24*60*60, "slot billing unit in seconds")
		slotsMinDuration   = flag.Int64("slot-min-duration-seconds", 7*24*60*60, "minimum slot lease duration in seconds")
		slotsMaxDuration   = flag.Int64("slot-max-duration-seconds", 90*24*60*60, "maximum slot lease duration in seconds")
	)
	flag.Parse()

	templates, err := buildPageTemplates(siteFS)
	if err != nil {
		log.Fatalf("template init: %v", err)
	}

	subStatic := mustSub(siteFS, "static")
	// http.FileServer expects an fs.FS wrapped with http.FS.
	var slotStore SlotStore = newMemorySlotStore()
	var slotCloser interface{ Close() error }
	if strings.EqualFold(strings.TrimSpace(*slotsStore), "chain") {
		cfg := chainSlotConfig{
			ChainID:            *chainID,
			NodeRPC:            *nodeRPC,
			GRPCAddr:           *slotsGRPC,
			KeyName:            *slotsKeyName,
			LeaseKeyName:       *leaseKeyName,
			KeyringBackend:     *keyringBackend,
			KeyringDir:         *keyringDir,
			Home:               *slotsHome,
			Fees:               *slotsFees,
			GasPrices:          *slotsGasPrices,
			Gas:                *slotsGas,
			GasAdjustment:      *slotsGasAdjustment,
			Binary:             *slotsBinary,
			RateDenom:          *slotsRateDenom,
			UnitSeconds:        *slotsUnitSeconds,
			MinDurationSeconds: *slotsMinDuration,
			MaxDurationSeconds: *slotsMaxDuration,
		}
		chainStore, err := newChainSlotStore(cfg)
		if err != nil {
			log.Fatalf("slot store init: %v", err)
		}
		slotStore = chainStore
		slotCloser = chainStore
	}

	s := &server{
		templates: templates,
		static:    http.FileServer(http.FS(subStatic)),
		slotStore: slotStore,
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))
	mux.HandleFunc("GET /", s.handleHome(*baseURL))
	mux.HandleFunc("GET /publishers", s.handlePublishers(*baseURL))
	mux.HandleFunc("GET /verifiers", s.handleVerifiers(*baseURL))
	mux.HandleFunc("GET /docs", s.handleDocs(*baseURL))
	mux.HandleFunc("GET /marketplace", s.handleMarketplace(*baseURL))
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
			DBPath:        *airdropDB,
			ChainID:       *chainID,
			NodeRPC:       *nodeRPC,
			Denom:         *denom,
			Amount:        *amount,
			FaucetKeyName: *faucetKeyName,
			Keyring:       *keyringBackend,
			Fees:          *fees,
			GasPrices:     *gasPrices,
			BaseURL:       *baseURL,
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
	Title       string
	Description string
	BaseURL     string
	Path        string
	NowYear     int
	Flash       string
}

func (s *server) handleHome(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, "home.html", pageData{
			Title:       "Congrid — Content Grid Protocol",
			Description: "A decentralized content network and search protocol. Become a Publisher or a Verifier to help build an open, community-owned discovery engine.",
			BaseURL:     baseURL,
			Path:        r.URL.Path,
			NowYear:     time.Now().Year(),
		})
	}
}

func (s *server) handlePublishers(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, "publishers.html", pageData{
			Title:       "Become a Publisher — Congrid",
			Description: "Register your site, add the Congrid verification badge, and earn rewards while sending high-quality referral traffic across the open web.",
			BaseURL:     baseURL,
			Path:        r.URL.Path,
			NowYear:     time.Now().Year(),
		})
	}
}

func (s *server) handleVerifiers(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, "verifiers.html", pageData{
			Title:       "Become a Verifier — Congrid",
			Description: "Run verifier software to help confirm publishers and earn a share of the network’s rewards.",
			BaseURL:     baseURL,
			Path:        r.URL.Path,
			NowYear:     time.Now().Year(),
		})
	}
}

func (s *server) handleDocs(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, "docs.html", pageData{
			Title:       "Docs — Congrid",
			Description: "Whitepaper, protocol overview, and contribution links.",
			BaseURL:     baseURL,
			Path:        r.URL.Path,
			NowYear:     time.Now().Year(),
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
