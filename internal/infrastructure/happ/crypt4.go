package happ

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"
)

const (
	crypt4Prefix = "happ://crypt4/"
	rsa4096Size  = 512
)

// publicKeyV4 — HAPP_PUBLIC_KEY_V4 из @kastov/cryptohapp (src/constants/crypt4.constant.ts).
// Публичный ключ Happ crypt4, не secret.
const publicKeyV4 = `-----BEGIN PUBLIC KEY-----
MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEA3UZ0M3L4K+WjM3vkbQnz
ozHg/cRbEXvQ6i4A8RVN4OM3rK9kU01FdjyoIgywve8OEKsFnVwERZAQZ1Trv60B
hmaM76QQEE+EUlIOL9EpwKWGtTL5lYC1sT9XJMNP3/CI0gP5wwQI88cY/xedpOEB
W72EmOOShHUm/b/3m+HPmqwc4ugKj5zWV5SyiT829aFA5DxSjmIIFBAms7DafmSq
LFTYIQL5cShDY2u+/sqyAw9yZIOoqW2TFIgIHhLPWek/ocDU7zyOrlu1E0SmcQQb
LFqHq02fsnH6IcqTv3N5Adb/CkZDDQ6HvQVBmqbKZKf7ZdXkqsc/Zw27xhG7OfXC
tUmWsiL7zA+KoTd3avyOh93Q9ju4UQsHthL3Gs4vECYOCS9dsXXSHEY/1ngU/hjO
WFF8QEE/rYV6nA4PTyUvo5RsctSQL/9DJX7XNh3zngvif8LsCN2MPvx6X+zLouBX
zgBkQ9DFfZAGLWf9TR7KVjZC/3NsuUCDoAOcpmN8pENBbeB0puiKMMWSvll36+2M
YR1Xs0MgT8Y9TwhE2+TnnTJOhzmHi/BxiUlY/w2E0s4ax9GHAmX0wyF4zeV7kDkc
vHuEdc0d7vDmdw0oqCqWj0Xwq86HfORu6tm1A8uRATjb4SzjTKclKuoElVAVa5Jo
oh/uZMozC65SmDw+N5p6Su8CAwEAAQ==
-----END PUBLIC KEY-----
`

func parseCrypt4PublicKey() (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyV4))
	if block == nil {
		return nil, fmt.Errorf("happ: failed to decode crypt4 public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("happ: parse crypt4 public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("happ: crypt4 public key is not RSA")
	}
	return rsaPub, nil
}

// CreateCrypt4Link шифрует subscription URL в Happ deep link happ://crypt4/<base64>.
// Алгоритм совпадает с @kastov/cryptohapp createHappCryptoLink(url, "v4", true).
func CreateCrypt4Link(subscriptionURL string) (string, error) {
	raw := strings.TrimSpace(subscriptionURL)
	if raw == "" {
		return "", fmt.Errorf("happ: empty subscription URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("happ: invalid subscription URL: %w", err)
	}
	if !u.IsAbs() || u.Host == "" {
		return "", fmt.Errorf("happ: subscription URL must be absolute")
	}

	pub, err := parseCrypt4PublicKey()
	if err != nil {
		return "", err
	}
	if pub.Size() != rsa4096Size {
		return "", fmt.Errorf("happ: unexpected RSA key size %d", pub.Size())
	}

	plain := []byte(raw)
	maxPlain := pub.Size() - 11
	if len(plain) > maxPlain {
		return "", fmt.Errorf("happ: subscription URL too long (%d bytes, max %d)", len(plain), maxPlain)
	}

	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, pub, plain)
	if err != nil {
		return "", fmt.Errorf("happ: encrypt: %w", err)
	}
	return crypt4Prefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}
