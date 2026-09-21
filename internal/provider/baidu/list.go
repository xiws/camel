package baidu

import (
	"context"
	"fmt"
	"net/url"

	"camel/internal/provider"
)

func (b *BaiduProvider) List(ctx context.Context, dir string) ([]provider.FileInfo, error) {
	params := url.Values{
		"dir":    {dir},
		"start":  {"0"},
		"limit":  {"100"},
		"order":  {"name"},
		"desc":   {"0"},
		"preset": {"0"},
	}

	data, err := b.api("list", params, nil)
	if err != nil {
		return nil, err
	}

	list, ok := data["list"].([]interface{})
	if !ok {
		return nil, nil
	}

	var files []provider.FileInfo
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		isDir := false
		if v, ok := m["isdir"]; ok {
			if n, ok := v.(float64); ok && n == 1 {
				isDir = true
			}
		}

		name := ""
		if v, ok := m["server_filename"]; ok {
			name, _ = v.(string)
		}
		if name == "" {
			if v, ok := m["path"]; ok {
				name, _ = v.(string)
			}
		}

		path := ""
		if v, ok := m["path"]; ok {
			path, _ = v.(string)
		}

		size := int64(0)
		if v, ok := m["size"]; ok {
			if n, ok := v.(float64); ok {
				size = int64(n)
			}
		}

		fsID := ""
		if v, ok := m["fs_id"]; ok {
			fsID = fmt.Sprintf("%v", v)
		}

		files = append(files, provider.FileInfo{
			ID:    fsID,
			Name:  name,
			Path:  path,
			Size:  size,
			IsDir: isDir,
		})
	}

	return files, nil
}
