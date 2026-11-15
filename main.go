package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

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

func renderQR(targetURL string) (string, error) {
	qrc, err := qrcode.New(targetURL, qrcode.Medium)
	if err != nil {
		return "", err
	}
	return qrc.ToString(false), nil
}

func main() {
	port := flag.String("port", "8085", "Web server port")
	days := flag.Int("days", 7, "Persistent mobile session lifetime in days")
	ntfyTopicFlag := flag.String("ntfy", "", "ntfy.sh topic name for iOS push alerts (defaults to auto-generated)")
	ntfyServer := flag.String("ntfy-server", "https://ntfy.sh", "ntfy server URL")
	telegramToken := flag.String("telegram-token", os.Getenv("TELEGRAM_BOT_TOKEN"), "Telegram Bot Token for iOS alerts")
	telegramChatID := flag.String("telegram-chat-id", os.Getenv("TELEGRAM_CHAT_ID"), "Telegram Chat ID for iOS alerts")
	flag.Parse()

	sessionToken := generateToken()
	localIP := getLocalIP()
	pairingURL := fmt.Sprintf("http://%s:%s/?token=%s", localIP, *port, sessionToken)
	adminURL := fmt.Sprintf("http://localhost:%s/", *port)

	// Configure ntfy topic
	ntfyTopic := *ntfyTopicFlag
	if ntfyTopic == "" {
		ntfyTopic = fmt.Sprintf("guaita_%s", sessionToken[:8])
	}

	// Initialize Multi-Channel Notification Engine
	dispatcher := notify.NewDispatcher()
	ntfyNotifier := notify.NewNtfy(*ntfyServer, ntfyTopic)
	dispatcher.Register(ntfyNotifier)

	webPushStore := notify.NewWebPushStore()
	dispatcher.Register(webPushStore)

	telegramActive := false
	if *telegramToken != "" && *telegramChatID != "" {
		tgNotifier := notify.NewTelegram(*telegramToken, *telegramChatID, func(action string) {
			_ = herdr.SendAction(action)
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
	fmt.Println()

	qrString, err := renderQR(pairingURL)
	if err == nil {
		fmt.Println(qrString)
	} else {
		log.Printf("Warning: failed to render terminal QR code: %v", err)
	}

	srv := server.New(server.Config{
		SessionToken:    sessionToken,
		SessionDays:     *days,
		LocalIP:         localIP,
		Port:            *port,
		Dispatcher:      dispatcher,
		WebPushStore:    webPushStore,
		NtfyTopic:       ntfyTopic,
		NtfyServer:      *ntfyServer,
		TelegramEnabled: telegramActive,
	})

	addr := fmt.Sprintf("0.0.0.0:%s", *port)
	log.Printf("Guaita listening on %s (Access control active)", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Routes()))
}
