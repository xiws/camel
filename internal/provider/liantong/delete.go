package liantong

import (
	"context"
	"fmt"
)

func (l *LiantongProvider) Delete(ctx context.Context, paths []string) error {
	fileList := make([]interface{}, 0)
	dirList := make([]interface{}, 0)

	for _, path := range paths {
		fileList = append(fileList, path)
	}

	params := map[string]interface{}{
		"spaceType": "0",
		"vipLevel":  "0",
		"dirList":   dirList,
		"fileList":  fileList,
	}

	_, err := l.dispatcher.Call("wohome", "DeleteFile", params)
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	return nil
}
