package baidu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

func (b *BaiduProvider) Delete(ctx context.Context, paths []string) error {
	fileListJSON, err := json.Marshal(paths)
	if err != nil {
		return err
	}

	form := url.Values{
		"filelist": {string(fileListJSON)},
	}

	params := url.Values{
		"opera": {"delete"},
		"async": {"0"},
		"ondup": {"1"},
	}

	data, err := b.api("filemanager", params, form)
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	if info, ok := data["info"].([]interface{}); ok {
		for _, item := range info {
			if m, ok := item.(map[string]interface{}); ok {
				errno := 0
				if v, ok := m["errno"]; ok {
					if n, ok := v.(float64); ok {
						errno = int(n)
					}
				}
				if errno != 0 && errno != -9 && errno != 31066 {
					return fmt.Errorf("delete failed for some items: %v", info)
				}
			}
		}
	}

	return nil
}
