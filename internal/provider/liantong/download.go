package liantong

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"camel/internal/provider"
)

func (l *LiantongProvider) Download(ctx context.Context, remotePath string, localPath string) error {
	fid, err := l.resolveFid(remotePath)
	if err != nil {
		return err
	}

	params := map[string]interface{}{
		"fidList":   []string{fid},
		"clientId":  clientID,
		"spaceType": "0",
	}

	_, err = l.dispatcher.Call("wohome", "GetDownloadUrl", params)
	if err != nil {
		return fmt.Errorf("GetDownloadUrl failed: %w", err)
	}

	dataArray, ok := l.dispatcher.lastRawData.([]interface{})
	if !ok {
		return fmt.Errorf("GetDownloadUrl returned unexpected format")
	}
	if len(dataArray) == 0 {
		return fmt.Errorf("GetDownloadUrl returned empty data")
	}

	firstItem, ok := dataArray[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid download URL format")
	}

	downloadURL, ok := firstItem["downloadUrl"].(string)
	if !ok {
		return fmt.Errorf("missing downloadUrl")
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("accesstoken", l.accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()

	cb := provider.FromContext(ctx)
	total := resp.ContentLength

	if cb == nil {
		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		return nil
	}

	buf := make([]byte, 32*1024)
	var written int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("failed to write file: %w", wErr)
			}
			written += int64(n)
			cb(written, total)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("failed to write file: %w", readErr)
		}
	}

	return nil
}

// resolveFid 优先把参数当成云盘路径（"/目录/文件.txt"）；
// 只有当路径解析不到东西时，才把它当成直接传入的 fid（长 base64 串）。
func (l *LiantongProvider) resolveFid(remotePath string) (string, error) {
	resolved, err := l.resolveEntry(remotePath)
	if err == nil {
		if resolved.entry.isDir {
			return "", fmt.Errorf("cannot download a directory: %s", remotePath)
		}
		if resolved.entry.fid == "" {
			return "", fmt.Errorf("no download id for: %s", remotePath)
		}
		return resolved.entry.fid, nil
	}

	if segments := splitSegments(remotePath); len(segments) == 1 && len(segments[0]) > 40 {
		return segments[0], nil
	}

	return "", err
}
