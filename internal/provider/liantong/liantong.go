package liantong

import (
	"context"
	"errors"
	"fmt"

	"camel/internal/provider"
)

type LiantongProvider struct {
	accessToken string
	phone       string
	dispatcher  *Dispatcher
}

func New() *LiantongProvider {
	return &LiantongProvider{
		dispatcher: NewDispatcher(),
	}
}

func (l *LiantongProvider) Name() string {
	return "lt"
}

func (l *LiantongProvider) Init(ctx context.Context, creds map[string]string) error {
	l.accessToken = creds["access_token"]
	l.phone = creds["phone"]
	if l.accessToken == "" {
		return errors.New("missing access_token in credentials")
	}
	l.dispatcher.SetAccessToken(l.accessToken)
	return nil
}

// List 列出目录内容。dir 可以是路径（"/备份"），也可以是目录 ID（"0"，兼容旧用法）。
func (l *LiantongProvider) List(ctx context.Context, dir string) ([]provider.FileInfo, error) {
	dirID, err := l.resolveDirID(dir)
	if err != nil {
		return nil, err
	}

	entries, err := l.listDir(dirID)
	if err != nil {
		return nil, err
	}

	files := make([]provider.FileInfo, 0, len(entries))
	for _, e := range entries {
		files = append(files, provider.FileInfo{
			ID:       e.id,
			Fid:      e.fid,
			Name:     e.name,
			Path:     displayPath(dir, e.name),
			Size:     e.size,
			IsDir:    e.isDir,
			FileType: e.fileType,
		})
	}

	return files, nil
}

func toStringValue(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%.0f", t)
	}
	return ""
}
