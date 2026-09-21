package liantong

import (
	"context"
	"fmt"
)

func (l *LiantongProvider) Move(ctx context.Context, src string, dest string, newName string) error {
	return fmt.Errorf("move not supported on liantong provider")
}

func (l *LiantongProvider) Copy(ctx context.Context, src string, dest string, newName string) error {
	return fmt.Errorf("copy not supported on liantong provider")
}

func (l *LiantongProvider) Mkdir(ctx context.Context, path string) error {
	return fmt.Errorf("mkdir not supported on liantong provider")
}

func (l *LiantongProvider) Touch(ctx context.Context, path string) error {
	return fmt.Errorf("touch not supported on liantong provider")
}
