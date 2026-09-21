package liantong

import (
	"context"
	"fmt"
)

// Delete 删除文件或目录。paths 是用户路径（"/文档/a.txt"），
// 内部解析成云盘 ID 后按 DeleteFile 的要求分到 fileList / dirList。
func (l *LiantongProvider) Delete(ctx context.Context, paths []string) error {
	fileList := make([]interface{}, 0, len(paths))
	dirList := make([]interface{}, 0, len(paths))

	for _, p := range paths {
		resolved, err := l.resolveEntry(p)
		if err != nil {
			return err
		}
		if resolved.entry.isDir {
			dirList = append(dirList, resolved.entry.id)
		} else {
			fileList = append(fileList, resolved.entry.id)
		}
	}

	params := map[string]interface{}{
		"spaceType": "0",
		"vipLevel":  "0",
		"dirList":   dirList,
		"fileList":  fileList,
	}

	if _, err := l.dispatcher.Call("wohome", "DeleteFile", params); err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	return nil
}
