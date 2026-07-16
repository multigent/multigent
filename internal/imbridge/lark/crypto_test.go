package lark

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestDecryptEncryptedEvent(t *testing.T) {
	plaintext := []byte(`{"schema":"2.0","header":{"event_type":"im.message.receive_v1","app_id":"cli_app"},"event":{"message":{"message_id":"om_1"}}}`)
	encrypted := encryptEventForTest(t, plaintext, "encrypt-one")

	decrypted, err := DecryptEncryptedEvent(encrypted, "encrypt-one")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted=%s", decrypted)
	}
	if _, err := DecryptEncryptedEvent(encrypted, "wrong-key"); err == nil {
		t.Fatalf("wrong key should fail")
	}
}

func encryptEventForTest(t *testing.T, plaintext []byte, encryptKey string) string {
	t.Helper()
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte(nil), plaintext...), make([]byte, padding)...)
	for i := len(padded) - padding; i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	iv := []byte("1234567890abcdef")
	ciphertext := append([]byte(nil), iv...)
	body := append([]byte(nil), padded...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(body, body)
	ciphertext = append(ciphertext, body...)
	return base64.StdEncoding.EncodeToString(ciphertext)
}
