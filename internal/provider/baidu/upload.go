package baidu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"camel/internal/provider"
)

func (b *BaiduProvider) Upload(ctx context.Context, localPath string, remoteDir string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("cannot access local file: %w", err)
	}

	remotePath := remoteDir + "/" + filepath.Base(localPath)
	if remoteDir == "/" {
		remotePath = "/" + filepath.Base(localPath)
	}

	size := info.Size()
	contentMD5, err := fileMD5(localPath)
	if err != nil {
		return fmt.Errorf("failed to compute file MD5: %w", err)
	}

	var blockList []string
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, blockSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			blockList = append(blockList, chunkMD5(buf[:n]))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if len(blockList) == 0 {
		blockList = append(blockList, chunkMD5([]byte{}))
	}

	blockListJSON, _ := json.Marshal(blockList)

	form := url.Values{
		"path":        {remotePath},
		"size":        {fmt.Sprintf("%d", size)},
		"isdir":       {"0"},
		"block_list":  {string(blockListJSON)},
		"rtype":       {"1"},
		"autoinit":    {"1"},
		"content-md5": {contentMD5},
	}

	pre, err := b.api("precreate", nil, form)
	if err != nil {
		return fmt.Errorf("precreate failed: %w", err)
	}

	returnType := 0
	if v, ok := pre["return_type"]; ok {
		if n, ok := v.(float64); ok {
			returnType = int(n)
		}
	}
	if returnType == 2 {
		return nil
	}

	uploadID, _ := pre["uploadid"].(string)
	if uploadID == "" {
		return fmt.Errorf("precreate response missing uploadid")
	}

	needUpload := []int{}
	if bl, ok := pre["block_list"].([]interface{}); ok {
		for _, v := range bl {
			if n, ok := v.(float64); ok {
				needUpload = append(needUpload, int(n))
			}
		}
	}

	if len(needUpload) > 0 {
		cb := provider.FromContext(ctx)
		var uploadedBytes int64

		locateParams := url.Values{
			"method":         {"locateupload"},
			"upload_version": {"2.0"},
			"app_id":         {appID},
		}
		if sign, ok := pre["uploadsign"]; ok {
			locateParams.Set("uploadsign", fmt.Sprintf("%v", sign))
		}

		raw, err := b.get(pcsFile, locateParams)
		if err != nil {
			return fmt.Errorf("locateupload failed: %w", err)
		}

		var located map[string]interface{}
		if err := json.Unmarshal(raw, &located); err != nil {
			return fmt.Errorf("failed to parse locateupload response: %w", err)
		}

		servers, _ := located["servers"].([]interface{})
		if len(servers) == 0 {
			return fmt.Errorf("no upload servers available")
		}

		server, _ := servers[0].(map[string]interface{})
		serverURL, _ := server["server"].(string)
		if serverURL == "" {
			return fmt.Errorf("invalid server URL")
		}

		f, err := os.Open(localPath)
		if err != nil {
			return err
		}
		defer f.Close()

		for _, idx := range needUpload {
			if _, err := f.Seek(int64(idx)*blockSize, 0); err != nil {
				return err
			}
			chunk := make([]byte, blockSize)
			n, err := f.Read(chunk)
			if err != nil && err != io.EOF {
				return err
			}
			chunk = chunk[:n]

			uploadParams := url.Values{
				"method":     {"upload"},
				"type":       {"tmpfile"},
				"path":       {remotePath},
				"partoffset": {fmt.Sprintf("%d", idx*blockSize)},
				"app_id":     {appID},
				"uploadid":   {uploadID},
				"partseq":    {fmt.Sprintf("%d", idx)},
			}

			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("file", "chunk")
			if err != nil {
				return err
			}
			if _, err := part.Write(chunk); err != nil {
				return err
			}
			writer.Close()

			uploadURL := serverURL + "/rest/2.0/pcs/superfile2?" + uploadParams.Encode()
			req, err := http.NewRequest("POST", uploadURL, &body)
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("Cookie", b.cookie())

			resp, err := b.client.Do(req)
			if err != nil {
				return fmt.Errorf("upload chunk %d failed: %w", idx, err)
			}
			resp.Body.Close()

			if cb != nil {
				uploadedBytes += int64(n)
				cb(uploadedBytes, size)
			}
		}
	}

	createForm := url.Values{
		"path":       {remotePath},
		"size":       {fmt.Sprintf("%d", size)},
		"isdir":      {"0"},
		"block_list": {string(blockListJSON)},
		"rtype":      {"1"},
		"uploadid":   {uploadID},
	}

	if _, err := b.api("create", nil, createForm); err != nil {
		return fmt.Errorf("create failed: %w", err)
	}

	return nil
}
