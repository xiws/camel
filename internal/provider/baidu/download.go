package baidu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"camel/internal/provider"
)

func (b *BaiduProvider) Download(ctx context.Context, remotePath string, localPath string) error {
	params := url.Values{
		"method":     {"locatedownload"},
		"path":       {remotePath},
		"ver":        {"2.0"},
		"dtype":      {"0"},
		"esl":        {"1"},
		"ehps":       {"1"},
		"app_id":     {appID},
		"check_blue": {"1"},
	}

	raw, err := b.get(pcsFile, params)
	if err != nil {
		return fmt.Errorf("locatedownload failed: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("failed to parse locatedownload response: %w", err)
	}

	urls, ok := data["urls"].([]interface{})
	if !ok || len(urls) == 0 {
		return fmt.Errorf("no download URLs available")
	}

	firstURL, ok := urls[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid URL format")
	}

	downloadURL, ok := firstURL["url"].(string)
	if !ok {
		return fmt.Errorf("missing download URL")
	}

	if len(downloadURL) >= 2 && downloadURL[:2] == "//" {
		downloadURL = "https:" + downloadURL
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Cookie", b.cookie())

	resp, err := b.dlClient.Do(req)
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
