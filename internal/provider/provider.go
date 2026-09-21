package provider

import "context"

type FileInfo struct {
	ID       string
	Fid      string
	Name     string
	Path     string
	Size     int64
	IsDir    bool
	FileType string
	Extra    map[string]interface{}
}

type LoginParams struct {
	Username string
	Password string
	BDUSS    string
}

// ProgressCallback reports transfer progress.
// bytesTransferred is the number of bytes transferred so far.
// total is the total number of bytes to transfer (-1 if unknown).
type ProgressCallback func(bytesTransferred, total int64)

type progressKey struct{}

func WithProgress(ctx context.Context, cb ProgressCallback) context.Context {
	return context.WithValue(ctx, progressKey{}, cb)
}

func FromContext(ctx context.Context) ProgressCallback {
	cb, _ := ctx.Value(progressKey{}).(ProgressCallback)
	return cb
}

type Provider interface {
	Name() string
	Login(ctx context.Context, params LoginParams) (map[string]string, error)
	Init(ctx context.Context, creds map[string]string) error
	List(ctx context.Context, dir string) ([]FileInfo, error)
	Upload(ctx context.Context, localPath string, remoteDir string) error
	Download(ctx context.Context, remotePath string, localPath string) error
	Delete(ctx context.Context, paths []string) error
	Move(ctx context.Context, src string, dest string, newName string) error
	Copy(ctx context.Context, src string, dest string, newName string) error
	Mkdir(ctx context.Context, path string) error
	Touch(ctx context.Context, path string) error
}
