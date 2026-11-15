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
		log.Fatalf("Error fatal al generar token criptográfico: %v", err)
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
	port := flag.String("port", "8085", "Puerto del servidor web")
	days := flag.Int("days", 7, "Días de validez de la sesión persistente en el móvil")
	flag.Parse()

	sessionToken := generateToken()
	localIP := getLocalIP()
	url := fmt.Sprintf("http://%s:%s/?token=%s", localIP, *port, sessionToken)

	fmt.Printf("\n🛡️ [Guaita 2026] Iniciado con persistencia de %d días.\n", *days)
	fmt.Printf("Conexión segura: %s\n\n", url)

	qrString, err := renderQR(url)
	if err == nil {
		fmt.Println(qrString)
	} else {
		log.Printf("Aviso: no se pudo dibujar el QR en la terminal: %v", err)
	}

	srv := server.New(server.Config{
		SessionToken: sessionToken,
		SessionDays:  *days,
	})

	addr := fmt.Sprintf("0.0.0.0:%s", *port)
	log.Printf("Guaita escuchando en %s (Control de acceso activo)", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Routes()))
}
