package lark

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type EncryptedEnvelope struct {
	Encrypt string `json:"encrypt"`
}

func ExtractEncryptedPayload(raw []byte) (string, bool) {
	var env EncryptedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", false
	}
	payload := strings.TrimSpace(env.Encrypt)
	return payload, payload != ""
}

func DecryptEncryptedEvent(encryptedPayload, encryptKey string) ([]byte, error) {
	encryptedPayload = strings.TrimSpace(encryptedPayload)
	encryptKey = strings.TrimSpace(encryptKey)
	if encryptedPayload == "" {
		return nil, fmt.Errorf("encrypted payload is required")
	}
	if encryptKey == "" {
		return nil, fmt.Errorf("encrypt key is required")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedPayload)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted payload: %w", err)
	}
	if len(ciphertext) < aes.BlockSize*2 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid encrypted payload length")
	}
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	iv := ciphertext[:aes.BlockSize]
	body := append([]byte(nil), ciphertext[aes.BlockSize:]...)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(body, body)
	body, err = pkcs7Unpad(body, aes.BlockSize)
	if err != nil {
		return nil, err
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("decrypted event is not JSON")
	}
	return body, nil
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid padded data length")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid padding")
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}
