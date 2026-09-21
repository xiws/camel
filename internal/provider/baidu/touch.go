package baidu

import (
	"context"
	"fmt"
	"net/url"
)

func (b *BaiduProvider) Touch(ctx context.Context, path string) error {
	form := url.Values{
		"path":       {path},
		"size":       {"0"},
		"isdir":      {"0"},
		"block_list": {"[]"},
	}

	params := url.Values{
		"a": {"commit"},
	}

	_, err := b.api("create", params, form)
	if err != nil {
		return fmt.Errorf("touch failed: %w", err)
	}
	return nil
}
