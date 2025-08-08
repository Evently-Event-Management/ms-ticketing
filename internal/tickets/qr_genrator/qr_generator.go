package qr

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"ms-ticketing/internal/models"

	"github.com/skip2/go-qrcode"
)

type QRGenerator struct {
	secret []byte
}

func NewQRGenerator(secret string) *QRGenerator {
	hashed := sha256.Sum256([]byte(secret)) // normalize to 32 bytes
	return &QRGenerator{secret: hashed[:]}
}

func (q *QRGenerator) GenerateEncryptedQR(ticket models.Ticket) ([]byte, error) {
	data, err := json.Marshal(ticket)
	if err != nil {
		return nil, err
	}

	encrypted, err := encryptAES(data, q.secret)
	if err != nil {
		return nil, err
	}

	return qrcode.Encode(encrypted, qrcode.Medium, 256)
}

func encryptAES(data []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	ciphertext := make([]byte, aes.BlockSize+len(data))
	iv := ciphertext[:aes.BlockSize]

	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], data)

	return base64.URLEncoding.EncodeToString(ciphertext), nil
}
