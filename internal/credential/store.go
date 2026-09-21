package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var fixedKey = []byte("camel-cli-encryption-key-32bytes")

type Credentials map[string]string

func Save(provider string, creds Credentials) error {
	dir, err := getCamelDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.Marshal(creds)
	if err != nil {
		return err
	}

	encrypted, err := encrypt(data)
	if err != nil {
		return err
	}

	filePath := filepath.Join(dir, provider+".enc")
	return os.WriteFile(filePath, encrypted, 0600)
}

func Load(provider string) (Credentials, error) {
	dir, err := getCamelDir()
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(dir, provider+".enc")
	encrypted, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	data, err := decrypt(encrypted)
	if err != nil {
		return nil, err
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return creds, nil
}

func getCamelDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".camel"), nil
}

func encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(fixedKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(fixedKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
