package liantong

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

const (
	dispatcherBase = "https://panservice.mail.wo.cn"
	clientID       = "1001000021"
	secretKey      = "XFmi9GS2hzk98jGX"
	aesIV          = "wNSOYIB1k1DjY5lA"
)

type Dispatcher struct {
	accessToken string
	client      *http.Client
	lastRawData interface{}
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (d *Dispatcher) SetAccessToken(token string) {
	d.accessToken = token
}

func (d *Dispatcher) Call(channel, operation string, params interface{}) (map[string]interface{}, error) {
	resTime := time.Now().UnixMilli()
	reqSeq := rand.Intn(90000) + 100000
	version := ""

	sign := computeSign(operation, resTime, reqSeq, channel, version)

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	var encrypted string
	var respKey []byte
	var bodyMap map[string]interface{}

	if channel == "api-user" {
		encrypted, err = aesEncrypt(paramsJSON, []byte(secretKey[:16]), []byte(aesIV))
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt params: %w", err)
		}
		respKey = nil
		bodyMap = map[string]interface{}{
			"param":    encrypted,
			"clientId": clientID,
			"secret":   true,
		}
	} else {
		var paramsCopy map[string]interface{}
		if err := json.Unmarshal(paramsJSON, &paramsCopy); err != nil {
			return nil, err
		}
		paramsCopy["clientId"] = clientID
		enriched, err := json.Marshal(paramsCopy)
		if err != nil {
			return nil, err
		}
		key := secretKey
		if d.accessToken != "" {
			key = d.accessToken
		}
		respKey = []byte(key[:16])
		encrypted, err = aesEncrypt(enriched, respKey, []byte(aesIV))
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt params: %w", err)
		}
		bodyMap = map[string]interface{}{
			"param": encrypted,
			"key":   true,
		}
	}

	body := map[string]interface{}{
		"header": map[string]interface{}{
			"key":     operation,
			"resTime": resTime,
			"reqSeq":  reqSeq,
			"channel": channel,
			"sign":    sign,
			"version": version,
		},
		"body": bodyMap,
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := dispatcherBase + "/" + channel + "/dispatcher"
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("accesstoken", d.accessToken)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dispatcher request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	rsp, ok := result["RSP"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("response missing RSP field")
	}

	rspCode, _ := rsp["RSP_CODE"].(string)
	if rspCode != "0000" {
		rspDesc, _ := rsp["RSP_DESC"].(string)
		return nil, fmt.Errorf("dispatcher error (code=%s): %s", rspCode, rspDesc)
	}

	if respKey != nil {
		if dataStr, ok := rsp["DATA"].(string); ok && dataStr != "" {
			decrypted, err := aesDecrypt(dataStr, respKey, []byte(aesIV))
			if err == nil {
				var decryptedData interface{}
				if err := json.Unmarshal(decrypted, &decryptedData); err == nil {
					rsp["DATA"] = decryptedData
				}
			}
		}
	}

	d.lastRawData = rsp["DATA"]

	data, ok := rsp["DATA"].(map[string]interface{})
	if !ok {
		data = map[string]interface{}{}
	}

	return data, nil
}

func computeSign(key string, resTime int64, reqSeq int, channel, version string) string {
	raw := fmt.Sprintf("%s%d%d%s%s", key, resTime, reqSeq, channel, version)
	h := md5.Sum([]byte(raw))
	return hex.EncodeToString(h[:])
}

func aesEncrypt(plaintext, key, iv []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	plaintext = pkcs7Pad(plaintext, aes.BlockSize)

	mode := cipher.NewCBCEncrypter(block, iv)
	ciphertext := make([]byte, len(plaintext))
	mode.CryptBlocks(ciphertext, plaintext)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func aesDecrypt(ciphertextB64 string, key, iv []byte) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	return pkcs7Unpad(plaintext, aes.BlockSize)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := make([]byte, len(data)+padding)
	copy(padtext, data)
	for i := len(data); i < len(padtext); i++ {
		padtext[i] = byte(padding)
	}
	return padtext
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid padding")
	}

	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize {
		return nil, fmt.Errorf("invalid padding size")
	}

	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return data[:len(data)-padding], nil
}
