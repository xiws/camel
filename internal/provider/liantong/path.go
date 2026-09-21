package liantong

import (
	"fmt"
	"strings"
)

// rootDirID 是联通云盘个人空间的根目录 ID。
const rootDirID = "0"

// pageSize 是列目录时的分页大小（当前实现只取第一页）。
const listPageSize = 200

// fileEntry 是 QueryAllFiles 返回项在 provider 内部的最小投影。
type fileEntry struct {
	id       string // 32 位 hex，重命名/移动/删除用
	fid      string // 长 base64 串，取下载地址用
	name     string
	size     int64
	fileType string // 0=全部 1=图片 2=视频 3=音频 4=文档 5=其他
	isDir    bool
	rawType  float64 // 列表原始 type：1=文件 0=目录
}

// listDir 列出某个目录的直接子项。dirID 必须是云盘目录 ID（根目录为 "0"）。
func (l *LiantongProvider) listDir(dirID string) ([]fileEntry, error) {
	if dirID == "" {
		dirID = rootDirID
	}

	params := map[string]interface{}{
		"spaceType":         "0",
		"parentDirectoryId": dirID,
		"pageNum":           0,
		"pageSize":          listPageSize,
		"sortRule":          0,
	}

	data, err := l.dispatcher.Call("wohome", "QueryAllFiles", params)
	if err != nil {
		return nil, err
	}

	filesRaw, _ := data["files"].([]interface{})
	entries := make([]fileEntry, 0, len(filesRaw))
	for _, item := range filesRaw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := toStringValue(m["name"])
		if name == "" {
			continue
		}
		rawType, _ := m["type"].(float64)
		var size int64
		switch t := m["size"].(type) {
		case float64:
			size = int64(t)
		case string:
			fmt.Sscanf(t, "%d", &size)
		}
		entries = append(entries, fileEntry{
			id:       toStringValue(m["id"]),
			fid:      toStringValue(m["fid"]),
			name:     name,
			size:     size,
			fileType: toStringValue(m["fileType"]),
			isDir:    rawType == 0,
			rawType:  rawType,
		})
	}
	return entries, nil
}

// childByName 在指定目录下按名字查找直接子项，找不到返回 (nil, nil)。
func (l *LiantongProvider) childByName(dirID, name string) (*fileEntry, error) {
	entries, err := l.listDir(dirID)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].name == name {
			return &entries[i], nil
		}
	}
	return nil, nil
}

// splitSegments 把用户路径拆成非空路径段。"/"、"//"、"" 都得到空切片。
func splitSegments(p string) []string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return nil
	}
	segments := make([]string, 0, 4)
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." {
			continue
		}
		segments = append(segments, seg)
	}
	return segments
}

// isIDLike 判断一个路径段是否形似云盘目录 ID：根目录 "0" 或 32 位 hex。
// 用于兼容 `camel list lt /0` 这类直接传 ID 的旧用法。
func isIDLike(s string) bool {
	if s == rootDirID {
		return true
	}
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// resolveDirID 把用户路径解析成云盘目录 ID。
//
//	"/"、""            -> 根目录 "0"
//	"/a/b"             -> 从根目录逐段按名字下钻
//	"0"、32 位 hex ID  -> 直接当目录 ID 使用（兼容旧用法）
func (l *LiantongProvider) resolveDirID(path string) (string, error) {
	segments := splitSegments(path)
	if len(segments) == 0 {
		return rootDirID, nil
	}
	if len(segments) == 1 && isIDLike(segments[0]) {
		return segments[0], nil
	}

	current := rootDirID
	for i, seg := range segments {
		entry, err := l.childByName(current, seg)
		if err != nil {
			return "", err
		}
		walked := "/" + strings.Join(segments[:i+1], "/")
		if entry == nil {
			return "", fmt.Errorf("directory not found: %s", walked)
		}
		if !entry.isDir {
			return "", fmt.Errorf("not a directory: %s", walked)
		}
		current = entry.id
	}
	return current, nil
}

// resolveParent 解析「待创建的目标」：返回其父目录 ID 与最后一段名字，不要求目标已存在。
func (l *LiantongProvider) resolveParent(path string) (parentID, name string, err error) {
	segments := splitSegments(path)
	if len(segments) == 0 {
		return "", "", fmt.Errorf("invalid path: %s", path)
	}
	name = segments[len(segments)-1]
	parentID, err = l.resolveDirID("/" + strings.Join(segments[:len(segments)-1], "/"))
	if err != nil {
		return "", "", err
	}
	return parentID, name, nil
}

// joinRemote 拼接目录路径与名字，用于生成错误信息里的可读路径。
func joinRemote(dir, name string) string {
	dir = strings.TrimSuffix(strings.TrimSpace(dir), "/")
	if dir == "" {
		return "/" + name
	}
	return dir + "/" + name
}

// displayPath 把「用户输入的目录」与「子项名字」拼成可读路径。
// 当目录是裸 ID（旧用法）时无法还原完整路径，退化为 "/名字"。
func displayPath(dir, name string) string {
	segments := splitSegments(dir)
	if len(segments) == 1 && isIDLike(segments[0]) {
		segments = nil
	}
	if len(segments) == 0 {
		return "/" + name
	}
	return "/" + strings.Join(segments, "/") + "/" + name
}

// resolvedEntry 是一个已存在于云盘上的条目的完整定位信息。
type resolvedEntry struct {
	entry    *fileEntry
	parentID string
}

// resolveEntry 把用户路径解析成云盘上的具体条目（文件或目录）。
func (l *LiantongProvider) resolveEntry(path string) (*resolvedEntry, error) {
	segments := splitSegments(path)
	if len(segments) == 0 {
		return nil, fmt.Errorf("path refers to the root directory: %s", path)
	}

	parentID, name, err := l.resolveParent(path)
	if err != nil {
		return nil, err
	}

	entry, err := l.childByName(parentID, name)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("no such file or directory: %s", path)
	}
	return &resolvedEntry{entry: entry, parentID: parentID}, nil
}
