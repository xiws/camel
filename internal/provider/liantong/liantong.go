package liantong

import (
	"context"
	"errors"

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

func (l *LiantongProvider) List(ctx context.Context, dir string) ([]provider.FileInfo, error) {
	parentDirID := "0"
	if dir != "/" && dir != "" {
		parentDirID = dir
	}

	params := map[string]interface{}{
		"spaceType":         "0",
		"parentDirectoryId": parentDirID,
		"pageNum":           0,
		"pageSize":          100,
		"sortRule":          0,
	}

	data, err := l.dispatcher.Call("wohome", "QueryAllFiles", params)
	if err != nil {
		return nil, err
	}

	filesRaw, ok := data["files"].([]interface{})
	if !ok {
		return nil, nil
	}

	var files []provider.FileInfo
	for _, item := range filesRaw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := m["name"].(string)
		id, _ := m["id"].(string)
		fid, _ := m["fid"].(string)
		size := int64(0)
		if v, ok := m["size"]; ok {
			switch t := v.(type) {
			case float64:
				size = int64(t)
			case string:
			}
		}
		fileType := ""
		if v, ok := m["fileType"]; ok {
			fileType, _ = v.(string)
		}
		typeVal, _ := m["type"].(float64)
		isDir := typeVal == 0

		parentDirId, _ := m["parentDirectoryId"].(string)

		files = append(files, provider.FileInfo{
			ID:       id,
			Fid:      fid,
			Name:     name,
			Path:     parentDirId + "/" + name,
			Size:     size,
			IsDir:    isDir,
			FileType: fileType,
			Extra:    m,
		})
	}

	return files, nil
}
