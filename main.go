package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

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
	flag.Parse()

	sessionToken := generateToken()
	localIP := getLocalIP()
	pairingURL := fmt.Sprintf("http://%s:%s/?token=%s", localIP, *port, sessionToken)
	adminURL := fmt.Sprintf("http://localhost:%s/", *port)

	fmt.Printf("\n🛡️ [Guaita 2026] Sentinel active with %d-day session persistence.\n", *days)
	fmt.Printf("🖥️  Localhost Admin:   %s\n", adminURL)
	fmt.Printf("📱 Mobile Pairing URL: %s\n\n", pairingURL)

	qrString, err := renderQR(pairingURL)
	if err == nil {
		fmt.Println(qrString)
	} else {
		log.Printf("Warning: failed to render terminal QR code: %v", err)
	}

	srv := server.New(server.Config{
		SessionToken: sessionToken,
		SessionDays:  *days,
		LocalIP:      localIP,
		Port:         *port,
	})

	addr := fmt.Sprintf("0.0.0.0:%s", *port)
	log.Printf("Guaita listening on %s (Access control active)", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Routes()))
}
