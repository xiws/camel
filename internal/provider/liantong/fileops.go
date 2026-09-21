package liantong

import (
	"context"
	"fmt"

	"camel/internal/provider"
)

// Move 支持两种语义：同目录改名（destDir 与源目录相同）和跨目录移动。
// destDir 是目标目录路径，newName 是目标文件名。
func (l *LiantongProvider) Move(ctx context.Context, src string, destDir string, newName string) error {
	resolved, err := l.resolveEntry(src)
	if err != nil {
		return err
	}
	if newName == "" {
		newName = resolved.entry.name
	}

	targetDirID, err := l.resolveDirID(destDir)
	if err != nil {
		return fmt.Errorf("invalid destination %q: %w", destDir, err)
	}
	if resolved.entry.isDir && targetDirID == resolved.entry.id {
		return fmt.Errorf("cannot move %s into itself", src)
	}

	// 目标位置已有同名条目时直接报错，避免依赖服务端的覆盖或自动改名行为。
	existing, err := l.childByName(targetDirID, newName)
	if err != nil {
		return err
	}
	if existing != nil && existing.id != resolved.entry.id {
		return fmt.Errorf("destination already exists: %s", joinRemote(destDir, newName))
	}

	// 同目录：只需要改名
	if targetDirID == resolved.parentID {
		if newName == resolved.entry.name {
			return nil
		}
		return l.renameEntry(resolved.entry, newName)
	}

	// 跨目录：先移动，再按需改名
	if err := l.moveEntry(resolved.entry, targetDirID); err != nil {
		return err
	}
	if newName != resolved.entry.name {
		if err := l.renameEntry(resolved.entry, newName); err != nil {
			return fmt.Errorf("moved, but renaming to %q failed: %w", newName, err)
		}
	}
	return nil
}

// Copy 把 src 复制到 destDir 下，newName 是副本名。
//
// 注意：联通在目标位置已存在同名文件时会自动改名（a.txt -> a(1).txt），
// 所以「复制成新名字」是通过复制前后对比目标目录、找出新出现的条目再改名实现的。
func (l *LiantongProvider) Copy(ctx context.Context, src string, destDir string, newName string) error {
	resolved, err := l.resolveEntry(src)
	if err != nil {
		return err
	}
	if newName == "" {
		newName = resolved.entry.name
	}

	targetDirID, err := l.resolveDirID(destDir)
	if err != nil {
		return fmt.Errorf("invalid destination %q: %w", destDir, err)
	}
	if resolved.entry.isDir && targetDirID == resolved.entry.id {
		return fmt.Errorf("cannot copy %s into itself", src)
	}

	// 同目录同名复制 = 创建副本，交给服务端的自动命名规则（a.txt -> a(1).txt）。
	samePlaceSameName := targetDirID == resolved.parentID && newName == resolved.entry.name
	if !samePlaceSameName {
		existing, err := l.childByName(targetDirID, newName)
		if err != nil {
			return err
		}
		if existing != nil {
			return fmt.Errorf("destination already exists: %s", joinRemote(destDir, newName))
		}
	}

	// 复制前的目标目录快照，用于事后定位新产生的条目。
	// 排序稳定，因此分页外的新条目只会导致定位失败（安全），不会认错已有条目。
	before, err := l.listDir(targetDirID)
	if err != nil {
		return err
	}

	if err := l.copyEntry(resolved.entry, targetDirID); err != nil {
		return err
	}

	if newName == resolved.entry.name {
		return nil
	}

	created, err := l.findNewEntry(targetDirID, before)
	if err != nil {
		return err
	}
	if created == nil {
		return fmt.Errorf("copied, but the new entry could not be located in %q", destDir)
	}
	if err := l.renameEntry(created, newName); err != nil {
		return fmt.Errorf("copied, but renaming to %q failed: %w", newName, err)
	}
	return nil
}

// Mkdir 在云端新建目录。path 是完整目录路径，如 "/备份/2026"。
func (l *LiantongProvider) Mkdir(ctx context.Context, path string) error {
	parentID, name, err := l.resolveParent(path)
	if err != nil {
		return err
	}

	existing, err := l.childByName(parentID, name)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("already exists: %s", path)
	}

	params := map[string]interface{}{
		"isCouldRepeat":     "0",
		"spaceType":         "0",
		"parentDirectoryId": parentID,
		"directoryName":     name,
	}
	if _, err := l.dispatcher.Call("wohome", "CreateDirectory", params); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}
	return nil
}

// Touch 在联通上不可用：联通没有「新建空文件」接口，而唯一的上传入口
// upload2C 会拒绝 0 字节文件（实测四种字段组合均返回 HTTP 400 Bad Request）。
// 因此这里不做伪实现（例如上传 1 字节），而是如实报错。
func (l *LiantongProvider) Touch(ctx context.Context, path string) error {
	return fmt.Errorf("touch is not supported on liantong: the provider has no create-file API, and its upload endpoint rejects zero-byte files (HTTP 400): %s", path)
}

func (l *LiantongProvider) renameEntry(entry *fileEntry, newName string) error {
	params := map[string]interface{}{
		"spaceType": "0",
		"type":      int(entry.rawType),
		"fileType":  entry.fileType,
		"id":        entry.id,
		"name":      newName,
	}
	if _, err := l.dispatcher.Call("wohome", "RenameFileOrDirectory", params); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	return nil
}

func (l *LiantongProvider) moveEntry(entry *fileEntry, targetDirID string) error {
	params := targetListParams(entry, targetDirID)
	if _, err := l.dispatcher.Call("wohome", "MoveFile", params); err != nil {
		return fmt.Errorf("move failed: %w", err)
	}
	return nil
}

func (l *LiantongProvider) copyEntry(entry *fileEntry, targetDirID string) error {
	params := targetListParams(entry, targetDirID)
	if _, err := l.dispatcher.Call("wohome", "CopyFile", params); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}
	return nil
}

// targetListParams 组装 MoveFile / CopyFile 共用的参数：文件放 fileList，目录放 dirList。
func targetListParams(entry *fileEntry, targetDirID string) map[string]interface{} {
	params := map[string]interface{}{
		"targetDirId": targetDirID,
		"sourceType":  "0",
		"targetType":  "0",
		"dirList":     []interface{}{},
		"fileList":    []interface{}{},
	}
	if entry.isDir {
		params["dirList"] = []interface{}{entry.id}
	} else {
		params["fileList"] = []interface{}{entry.id}
	}
	return params
}

// findNewEntry 找出目录中不在 before 快照里的条目。
func (l *LiantongProvider) findNewEntry(dirID string, before []fileEntry) (*fileEntry, error) {
	known := make(map[string]struct{}, len(before))
	for _, e := range before {
		known[e.id] = struct{}{}
	}

	after, err := l.listDir(dirID)
	if err != nil {
		return nil, err
	}
	for i := range after {
		if _, ok := known[after[i].id]; !ok {
			return &after[i], nil
		}
	}
	return nil, nil
}

var _ provider.Provider = (*LiantongProvider)(nil)
