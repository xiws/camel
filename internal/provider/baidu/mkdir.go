package baidu

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

func (b *BaiduProvider) Mkdir(ctx context.Context, path string) error {
	now := strconv.FormatInt(time.Now().Unix(), 10)

	form := url.Values{
		"path":        {path},
		"size":        {"0"},
		"isdir":       {"1"},
		"local_ctime": {now},
		"local_mtime": {now},
		"block_list":  {"[]"},
	}

	_, err := b.api("create?a=commit", nil, form)
	if err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}
	return nil
}
