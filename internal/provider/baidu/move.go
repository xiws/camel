package baidu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

func (b *BaiduProvider) Move(ctx context.Context, src string, dest string, newName string) error {
	item := map[string]string{
		"path":    src,
		"dest":    dest,
		"newname": newName,
	}
	fileListJSON, err := json.Marshal([]map[string]string{item})
	if err != nil {
		return err
	}

	form := url.Values{
		"filelist": {string(fileListJSON)},
	}

	params := url.Values{
		"opera":  {"move"},
		"async":  {"2"},
		"onnest": {"1"},
		"ondup":  {"1"},
	}

	_, err = b.api("filemanager", params, form)
	if err != nil {
		return fmt.Errorf("move failed: %w", err)
	}
	return nil
}
