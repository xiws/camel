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
	"strings"
	"time"

	"camel/internal/provider"
)

// Upload 上传本地文件到远端目录。remoteDir 既可以是路径（"/备份"），
// 也可以是目录 ID（"0" 或 32 位 hex，兼容旧用法）。
func (l *LiantongProvider) Upload(ctx context.Context, localPath string, remoteDir string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("cannot access local file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("cannot upload a directory: %s", localPath)
	}

	directoryID, err := l.resolveDirID(remoteDir)
	if err != nil {
		return fmt.Errorf("invalid remote directory %q: %w", remoteDir, err)
	}

	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return l.uploadStream(ctx, f, info.Size(), filepath.Base(localPath), directoryID)
}

// uploadStream 走 upload2C 单分片上传。fileSize 为 0 时会得到一个空文件（touch 用）。
func (l *LiantongProvider) uploadStream(ctx context.Context, src io.Reader, fileSize int64, fileName, directoryID string) error {
	if directoryID == "" {
		directoryID = rootDirID
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

	uniqueID := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), randomString(6))
	batchNo := randomString(32)

	fileInfoMap := map[string]interface{}{
		"fileName":    fileName,
		"fileSize":    fileSize,
		"fileType":    fileTypeOf(fileName),
		"directoryId": directoryID,
		"batchNo":     batchNo,
		"spaceType":   "0",
	}
	fileInfoJSON, _ := json.Marshal(fileInfoMap)
	fileInfoEncrypted, err := aesEncryptWithToken(fileInfoJSON, l.accessToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt fileInfo: %w", err)
	}

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

	var reader io.Reader = src
	if cb := provider.FromContext(ctx); cb != nil && fileSize > 0 {
		reader = &progressReader{reader: src, total: fileSize, cb: cb}
	}
	if _, err := io.Copy(part, reader); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed (HTTP %d): %s", resp.StatusCode, truncateForError(string(respBody), 200))
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

// truncateForError 截断过长的响应体，避免错误信息里塞进整页 HTML。
func truncateForError(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
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

func fileTypeOf(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	switch ext {
	case "jpg", "png", "bmp", "jpeg", "tif", "tiff", "psd", "tga", "raw", "pcd", "heic", "ico", "livp", "webp", "gif":
		return "1"
	case "avi", "asf", "m4v", "dat", "3gp", "dv", "flv", "mkv", "webm", "mov", "ogv", "mp4", "rm", "swf", "ts", "vob", "wmv", "rmvb", "mpg":
		return "2"
	case "au", "ac3", "flac", "m4a", "mp2", "mp3", "wav", "wma", "ape", "mpc", "tta", "ogg", "amr", "aac", "aiff", "mka", "wv":
		return "3"
	case "txt", "doc", "docx", "ppt", "pptx", "xls", "xlsx", "pdf", "rtf", "hlp", "md", "text":
		return "4"
	default:
		return "5"
	}
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
