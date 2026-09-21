package liantong

import (
	"reflect"
	"testing"
)

func TestSplitSegments(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"/", nil},
		{"", nil},
		{"   ", nil},
		{"/a", []string{"a"}},
		{"a", []string{"a"}},
		{"a/", []string{"a"}},
		{"/a/b", []string{"a", "b"}},
		{"//a//b//", []string{"a", "b"}},
		{"/a/./b", []string{"a", "b"}},
		{"/中文 目录/文件.txt", []string{"中文 目录", "文件.txt"}},
	}
	for _, c := range cases {
		if got := splitSegments(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitSegments(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestIsIDLike(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"0", true},
		{"cd153049d07049dfbf8b60c1530dcd2c", true},
		{"CD153049D07049DFBF8B60C1530DCD2C", true},
		{"cd153049d07049dfbf8b60c1530dcd2", false},   // 31 位
		{"cd153049d07049dfbf8b60c1530dcd2cz", false}, // 33 位
		{"cd153049d07049dfbf8b60c1530dcdgz", false},  // 含非 hex 字符
		{"docs", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isIDLike(c.in); got != c.want {
			t.Errorf("isIDLike(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestJoinRemote(t *testing.T) {
	cases := []struct {
		dir, name, want string
	}{
		{"/", "x.txt", "/x.txt"},
		{"", "x.txt", "/x.txt"},
		{"/a", "x.txt", "/a/x.txt"},
		{"/a/", "x.txt", "/a/x.txt"},
		{"/a/b", "x.txt", "/a/b/x.txt"},
	}
	for _, c := range cases {
		if got := joinRemote(c.dir, c.name); got != c.want {
			t.Errorf("joinRemote(%q, %q) = %q, want %q", c.dir, c.name, got, c.want)
		}
	}
}

func TestDisplayPath(t *testing.T) {
	cases := []struct {
		dir, name, want string
	}{
		{"/", "x.txt", "/x.txt"},
		{"/a", "x.txt", "/a/x.txt"},
		{"/a/b/", "x.txt", "/a/b/x.txt"},
		// 裸 ID（旧用法）无法还原层级，退化成根目录样式
		{"0", "x.txt", "/x.txt"},
		{"cd153049d07049dfbf8b60c1530dcd2c", "x.txt", "/x.txt"},
	}
	for _, c := range cases {
		if got := displayPath(c.dir, c.name); got != c.want {
			t.Errorf("displayPath(%q, %q) = %q, want %q", c.dir, c.name, got, c.want)
		}
	}
}
