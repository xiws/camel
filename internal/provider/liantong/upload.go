package liantong

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"camel/internal/provider"
)

func (l *LiantongProvider) Upload(ctx context.Context, localPath string, remoteDir string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("cannot access local file: %w", err)
	}

	directoryID := "0"
	if remoteDir != "/" && remoteDir != "" {
		directoryID = remoteDir
	}

	zoneParams := map[string]interface{}{
		"appId": "10000001",
	}
	zoneData, err := l.dispatcher.Call("wohome", "GetZoneInfo", zoneParams)
	if err != nil {
		return fmt.Errorf("GetZoneInfo failed: %w", err)
	}
	uploadHost, _ := zoneData["url"].(string)
	if uploadHost == "" {
		uploadHost = "https://hyupload.pan.wo.cn"
	}

	fileName := filepath.Base(localPath)
	fileSize := info.Size()
	uniqueID := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), randomString(6))
	batchNo := randomString(32)

	fileInfoMap := map[string]interface{}{
		"batchNo":   batchNo,
		"spaceType": "0",
	}
	fileInfoJSON, _ := json.Marshal(fileInfoMap)
	fileInfoEncrypted, err := aesEncryptWithToken(fileInfoJSON, l.accessToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt fileInfo: %w", err)
	}

	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	writer.WriteField("uniqueId", uniqueID)
	writer.WriteField("accessToken", l.accessToken)
	writer.WriteField("fileName", fileName)
	writer.WriteField("fileSize", fmt.Sprintf("%d", fileSize))
	writer.WriteField("totalPart", "1")
	writer.WriteField("partSize", fmt.Sprintf("%d", fileSize))
	writer.WriteField("partIndex", "1")
	writer.WriteField("channel", "wocloud")
	writer.WriteField("directoryId", directoryID)
	writer.WriteField("psToken", "")
	writer.WriteField("fileInfo", fileInfoEncrypted)

	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return err
	}

	cb := provider.FromContext(ctx)
	var src io.Reader = f
	if cb != nil {
		src = &progressReader{reader: f, total: fileSize, cb: cb}
	}
	if _, err := io.Copy(part, src); err != nil {
		return err
	}
	writer.Close()

	uploadURL := uploadHost + "/openapi/client/upload2C"
	req, err := http.NewRequest("POST", uploadURL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("accesstoken", l.accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse upload response: %w", err)
	}

	code, _ := result["code"].(string)
	if code != "0000" {
		msg, _ := result["msg"].(string)
		return fmt.Errorf("upload failed (code=%s): %s", code, msg)
	}

	return nil
}

func aesEncryptWithToken(plaintext []byte, token string) (string, error) {
	key := make([]byte, 16)
	copy(key, token)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	plaintext = pkcs7Pad(plaintext, aes.BlockSize)
	iv := []byte(aesIV)

	mode := cipher.NewCBCEncrypter(block, iv)
	ciphertext := make([]byte, len(plaintext))
	mode.CryptBlocks(ciphertext, plaintext)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func randomString(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[r.Intn(len(chars))]
	}
	return string(b)
}

type progressReader struct {
	reader    io.Reader
	total     int64
	cb        provider.ProgressCallback
	readSoFar int64
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.readSoFar += int64(n)
		pr.cb(pr.readSoFar, pr.total)
	}
	return n, err
}
