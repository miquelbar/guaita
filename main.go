package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"guaita/config"
	"guaita/herdr"
	"guaita/notify"
	"guaita/server"
	"github.com/skip2/go-qrcode"
)

func generateToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatalf("Fatal error generating cryptographic token: %v", err)
	}
	return hex.EncodeToString(bytes)
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

// renderQR renders a compact terminal QR code using Unicode half-blocks (2 rows per line).
func renderQR(targetURL string) (string, error) {
	qrc, err := qrcode.New(targetURL, qrcode.Low)
	if err != nil {
		return "", err
	}
	qrc.DisableBorder = true
	bitmap := qrc.Bitmap()
	n := len(bitmap)
	if n == 0 {
		return "", nil
	}

	const border = 1
	size := n + border*2
	isLight := func(x, y int) bool {
		bx, by := x-border, y-border
		if bx < 0 || by < 0 || bx >= n || by >= n {
			return true
		}
		return !bitmap[by][bx]
	}

	var b strings.Builder
	for y := 0; y < size; y += 2 {
		for x := 0; x < size; x++ {
			top := isLight(x, y)
			bot := true
			if y+1 < size {
				bot = isLight(x, y+1)
			}
			switch {
			case top && bot:
				b.WriteRune('█')
			case top && !bot:
				b.WriteRune('▀')
			case !top && bot:
				b.WriteRune('▄')
			default:
				b.WriteRune(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func generateSelfSignedCert(localIP string) (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	ipAddrs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	if parsed := net.ParseIP(localIP); parsed != nil {
		ipAddrs = append(ipAddrs, parsed)
	}
	tmpl := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "Guaita Local Sentinel"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(30 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  ipAddrs,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}, nil
}

func main() {
	defaultCfgPath, _ := config.DefaultPath()
	configFlag := flag.String("config", defaultCfgPath, "Path to persistent JSON configuration file")
	port := flag.String("port", "8085", "Web server port")
	days := flag.Int("days", 7, "Persistent mobile session lifetime in days")
	useTLS := flag.Bool("tls", true, "Enable HTTPS/WSS with an auto-generated self-signed certificate (use -tls=false for plain HTTP)")
	tlsCert := flag.String("tls-cert", "", "Path to TLS certificate file (implies -tls)")
	tlsKey := flag.String("tls-key", "", "Path to TLS private key file (implies -tls)")
	publicURLFlag := flag.String("public-url", "", "External public URL (e.g. Cloudflare Tunnel or reverse proxy URL)")
	adminAuthFlag := flag.Bool("admin-auth", true, "Require an admin token on localhost dashboard (set -admin-auth=false or admin_auth:false in config.json to disable)")
	adminTokenFlag := flag.String("admin-token", "", "Custom localhost admin token (auto-generated when -admin-auth is enabled)")
	ntfyTopicFlag := flag.String("ntfy", "", "ntfy.sh topic name for iOS push alerts (defaults to auto-generated)")
	ntfyServer := flag.String("ntfy-server", "https://ntfy.sh", "ntfy server URL")
	telegramToken := flag.String("telegram-token", "", "Telegram Bot Token for iOS alerts")
	telegramChatID := flag.String("telegram-chat-id", "", "Telegram Chat ID for iOS alerts")
	flag.Parse()

	explicitFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	cfgPath := *configFlag
	fileCfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("Warning: failed to load config file %s: %v", cfgPath, err)
	}

	// Merge precedence: defaults -> config.json -> env vars -> explicit CLI flags
	if !explicitFlags["port"] && fileCfg.Port != "" {
		*port = fileCfg.Port
	}
	if !explicitFlags["days"] && fileCfg.SessionDays > 0 {
		*days = fileCfg.SessionDays
	}
	if !explicitFlags["tls"] && fileCfg.TLS != nil {
		*useTLS = *fileCfg.TLS
	}
	if !explicitFlags["admin-auth"] && fileCfg.AdminAuth != nil {
		*adminAuthFlag = *fileCfg.AdminAuth
	}
	if !explicitFlags["tls-cert"] && fileCfg.TLSCert != "" {
		*tlsCert = fileCfg.TLSCert
	}
	if !explicitFlags["tls-key"] && fileCfg.TLSKey != "" {
		*tlsKey = fileCfg.TLSKey
	}
	if !explicitFlags["ntfy"] && fileCfg.NtfyTopic != "" {
		*ntfyTopicFlag = fileCfg.NtfyTopic
	}
	if !explicitFlags["ntfy-server"] && fileCfg.NtfyServer != "" {
		*ntfyServer = fileCfg.NtfyServer
	}
	if !explicitFlags["telegram-token"] {
		if env := os.Getenv("TELEGRAM_BOT_TOKEN"); env != "" {
			*telegramToken = env
		} else if fileCfg.TelegramBotToken != "" {
			*telegramToken = fileCfg.TelegramBotToken
		}
	}
	if !explicitFlags["telegram-chat-id"] {
		if env := os.Getenv("TELEGRAM_CHAT_ID"); env != "" {
			*telegramChatID = env
		} else if fileCfg.TelegramChatID != "" {
			*telegramChatID = fileCfg.TelegramChatID
		}
	}

	tlsEnabled := *useTLS || (*tlsCert != "" && *tlsKey != "")
	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}

	sessionToken := generateToken()
	localIP := getLocalIP()
	pairingURL := fmt.Sprintf("%s://%s:%s/?token=%s", scheme, localIP, *port, sessionToken)

	adminToken := ""
	adminURL := fmt.Sprintf("%s://localhost:%s/", scheme, *port)
	if *adminAuthFlag {
		adminToken = *adminTokenFlag
		if adminToken == "" {
			adminToken = generateToken()
		}
		adminURL = fmt.Sprintf("%s://localhost:%s/?admin_token=%s", scheme, *port, adminToken)
	}

	// Configure ntfy topic independently from sessionToken
	ntfyTopic := *ntfyTopicFlag
	if ntfyTopic == "" {
		ntfyTopic = fmt.Sprintf("guaita_%s", generateToken()[:16])
	}

	// Initialize Multi-Channel Notification Engine
	dispatcher := notify.NewDispatcher()
	ntfyNotifier := notify.NewNtfy(*ntfyServer, ntfyTopic)
	dispatcher.Register(ntfyNotifier)

	webPushStore := notify.NewWebPushStore()
	dispatcher.Register(webPushStore)

	telegramActive := false
	var tgNotifier *notify.TelegramNotifier
	if *telegramToken != "" && *telegramChatID != "" {
		tgNotifier = notify.NewTelegram(*telegramToken, *telegramChatID, func(rawAction string) {
			if paneID, act, ok := notify.ParseTelegramAction(rawAction); ok && paneID != "" {
				_ = herdr.SendActionToPane(paneID, act)
				return
			}
			_ = herdr.SendAction(rawAction)
		})
		dispatcher.Register(tgNotifier)
		telegramActive = true
	}

	fmt.Printf("\n🛡️ [Guaita 2026] Sentinel active with %d-day session persistence.\n", *days)
	fmt.Printf("🖥️  Localhost Admin:   %s\n", adminURL)
	fmt.Printf("📱 Mobile Pairing URL: %s\n", pairingURL)
	fmt.Printf("🔔 ntfy.sh iOS Push:   %s/%s\n", *ntfyServer, ntfyTopic)
	if telegramActive {
		fmt.Printf("💬 Telegram Bot Alert: Active\n")
	}
	if !tlsEnabled && !strings.HasPrefix(*publicURLFlag, "https://") {
		fmt.Println("⚠️  WARNING: Running over plain HTTP without TLS. LAN Wi-Fi traffic is unencrypted; use -tls or an HTTPS tunnel (-public-url https://...) on untrusted networks.")
	}
	fmt.Println()

	qrString, err := renderQR(pairingURL)
	if err == nil {
		fmt.Println(qrString)
	} else {
		log.Printf("Warning: failed to render terminal QR code: %v", err)
	}

	srv := server.New(server.Config{
		SessionToken:     sessionToken,
		AdminToken:       adminToken,
		SessionDays:      *days,
		LocalIP:          localIP,
		Port:             *port,
		PublicURL:        *publicURLFlag,
		TLSEnabled:       tlsEnabled,
		Dispatcher:       dispatcher,
		WebPushStore:     webPushStore,
		TelegramNotifier: tgNotifier,
		NtfyTopic:        ntfyTopic,
		NtfyServer:       *ntfyServer,
		TelegramEnabled:  telegramActive,
		ConfigPath:       cfgPath,
	})

	addr := fmt.Sprintf("0.0.0.0:%s", *port)
	go srv.StartMonitor(nil)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if tlsEnabled {
		log.Printf("Guaita listening on %s (TLS + Access control active)", addr)
		if *tlsCert != "" && *tlsKey != "" {
			log.Fatal(httpServer.ListenAndServeTLS(*tlsCert, *tlsKey))
		}
		cert, err := generateSelfSignedCert(localIP)
		if err != nil {
			log.Fatalf("Failed to generate self-signed TLS certificate: %v", err)
		}
		httpServer.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		log.Fatal(httpServer.ListenAndServeTLS("", ""))
	}

	log.Printf("Guaita listening on %s (Access control active)", addr)
	log.Fatal(httpServer.ListenAndServe())
}
