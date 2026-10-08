package netx

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"time"
)

// CertPin — pin-sha256 публичного ключа сертификата сервера, как ждёт
// openconnect --servercert. Сертификат не проверяем: его показывают
// человеку на подтверждение.
func CertPin(host, ip string) (string, error) {
	if ip == "" {
		ip = host
	}
	d := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", net.JoinHostPort(ip, "443"), &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return "", fmt.Errorf("не удалось получить сертификат %s: %v", host, err)
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", fmt.Errorf("%s не прислал сертификат", host)
	}
	sum := sha256.Sum256(certs[0].RawSubjectPublicKeyInfo)
	return "pin-sha256:" + base64.StdEncoding.EncodeToString(sum[:]), nil
}
