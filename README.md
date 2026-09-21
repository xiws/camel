# Camel - 多云盘统一命令行工具

Camel 是一个基于 Go 语言开发的命令行云存储管理工具，支持**百度网盘**和**联通沃云盘**两个云盘服务商。通过统一的 CLI 接口，用户可以方便地对多个云盘进行登录、文件浏览、上传、下载和删除等操作。

## 功能特性

- **多服务商支持** - 统一接口管理百度网盘 (`baidu`) 和联通沃云盘 (`lt`)
- **完整的文件操作** - 支持登录认证、文件列表浏览、文件上传、文件下载、文件删除、移动/重命名、复制、创建文件、创建目录
- **凭证安全存储** - 登录凭证使用 AES-GCM 加密后存储在本地 `~/.camel/` 目录
- **百度网盘** - 支持用户名/密码登录（RSA 加密）和 BDUSS Token 直接登录，分片上传、秒传检测、定位下载
- **联通沃云盘** - 支持手机号/密码登录（含图形验证码和短信验证码流程），Dispatcher 协议通信，AES-CBC 加密参数传输

## 项目结构

```
camel/
├── cmd/camel/                  # 程序入口
│   └── main.go
├── internal/
│   ├── cli/                    # CLI 命令定义 (基于 cobra)
│   │   ├── root.go             # 根命令与服务商注册
│   │   ├── login.go            # 登录命令
│   │   ├── list.go             # 文件列表命令
│   │   ├── upload.go           # 上传命令 (支持 glob 通配符)
│   │   ├── down.go             # 下载命令
│   │   ├── del.go              # 删除命令
│   │   ├── mv.go               # 移动/重命名命令
│   │   ├── cp.go               # 复制命令
│   │   ├── touch.go            # 创建空文件命令
│   │   └── mkdir.go            # 创建目录命令
│   ├── credential/             # 凭证加密存储
│   │   └── store.go            # AES-GCM 加密/解密，存储到 ~/.camel/
│   └── provider/               # 云盘服务商实现
│       ├── provider.go         # Provider 统一接口定义
│       ├── baidu/              # 百度网盘实现
│       │   ├── api.go          # API 基础层 (HTTP 请求、签名、RSA 加密)
│       │   ├── auth.go         # 登录认证 (账密 + BDUSS)
│       │   ├── list.go         # 文件列表
│       │   ├── upload.go       # 分片上传 (precreate → locateupload → superfile2 → create)
│       │   ├── download.go     # 定位下载 (locatedownload)
│       │   ├── delete.go       # 文件删除 (filemanager?opera=delete)
│       │   ├── move.go         # 文件移动 (filemanager?opera=move)
│       │   ├── copy.go         # 文件复制 (filemanager?opera=copy)
│       │   ├── mkdir.go        # 创建目录 (create?a=commit, isdir=1)
│       │   └── touch.go        # 创建空文件 (create?a=commit, isdir=0)
│       └── liantong/           # 联通沃云盘实现
│           ├── liantong.go     # Provider 主体 & 文件列表
│           ├── auth.go         # 登录认证 (密码 + 图形验证码 + 短信验证码)
│           ├── dispatcher.go   # Dispatcher 协议层 (签名、AES-CBC 加密通信)
│           ├── upload.go       # 文件上传 (upload2C)
│           ├── download.go     # 文件下载 (GetDownloadUrl)
│           ├── delete.go       # 文件删除 (DeleteFile)
│           └── fileops.go      # 文件操作桩 (mv/cp/touch/mkdir 暂未支持)
├── web-demo/                   # 百度网盘 Web 端 API 实验 (Node.js)
├── docs/                       # 逆向分析文档
│   ├── baidu/                  # 百度网盘 APK 逆向 API 文档
│   └── liantong/               # 联通沃云盘 APK 逆向分析文档
├── go.mod
├── go.sum
└── makefile
```

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.24 |
| CLI 框架 | [cobra](https://github.com/spf13/cobra) |
| 凭证加密 | AES-256-GCM |
| 百度网盘认证 | BDUSS Cookie + bdstoken (MD5) + RSA 密码加密 |
| 联通沃云盘通信 | Dispatcher 统一端点 + MD5 签名 + AES-CBC 参数加密 |

## 快速开始

### 编译

```bash
make build
# 产出 bin/camel
```

### 登录

```bash
# 百度网盘 - 使用 BDUSS Token 登录
./bin/camel login baidu --bduss "你的BDUSS"

# 百度网盘 - 使用用户名密码登录
./bin/camel login baidu -u "手机号/邮箱" -p "密码"

# 联通沃云盘 - 手机号密码登录（可能触发图形验证码和短信验证）
./bin/camel login lt -u "手机号" -p "密码"
```

### 文件操作

```bash
# 列出根目录文件
./bin/camel list baidu /
./bin/camel list lt /

# 上传文件（支持 glob 通配符）
./bin/camel upload baidu /备份 ./file1.txt ./file2.txt
./bin/camel upload lt /0 ./photo.jpg

# 下载文件
./bin/camel down baidu /文档/report.pdf -o ./report.pdf
./bin/camel down lt /fid123 -o ./output.txt

# 删除文件
./bin/camel del baidu /旧文件1 /旧文件2
./bin/camel del lt /fid1 /fid2

# 移动/重命名文件
./bin/camel mv baidu /文档/old.txt /文档/new.txt
./bin/camel mv baidu /文件.txt /备份/文件.txt

# 复制文件
./bin/camel cp baidu /文档/report.pdf /备份/report.pdf

# 创建空文件
./bin/camel touch baidu /文档/newfile.txt

# 创建目录
./bin/camel mkdir baidu /新建文件夹
./bin/camel mkdir baidu /文档/子目录
```

## 架构设计

### Provider 统一接口

所有云盘服务商实现 `provider.Provider` 接口：

```go
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
```

### 百度网盘上传流程

```
PreCreate (预创建，获取 uploadid)
    → LocateUpload (定位上传服务器)
    → SuperFile2 Upload (分片上传，4MB/片)
    → Create (合并创建文件)
```

### 联通沃云盘 Dispatcher 协议

所有业务操作通过统一的 `POST /{channel}/dispatcher` 端点，请求体结构：

```json
{
  "header": { "key": "操作名", "resTime": 时间戳, "reqSeq": 序列号, "sign": "MD5签名" },
  "body": { "param": "AES-CBC加密的参数JSON", "clientId": "客户端ID", "secret": true }
}
```

## 凭证存储

登录凭证加密存储在 `~/.camel/` 目录：

- `baidu.enc` - 百度网盘凭证（BDUSS、STOKEN）
- `lt.enc` - 联通沃云盘凭证（access_token、手机号）

加密方案：AES-GCM，密钥派生自固定 32 字节密钥，每次加密使用随机 nonce。

## 逆向工程文档

`docs/` 目录包含两个云盘的完整逆向分析文档：

- **百度网盘** (`docs/baidu/`) - 基于 APK v9.15.1.32 反编译分析，涵盖认证体系、API 域名体系、上传/下载/文件管理接口、RSA 加密算法、Frida Hook 示例
- **联通沃云盘** (`docs/liantong/`) - 基于 APK 逆向分析，涵盖梆梆加固绕过、Dispatcher 协议确认、签名/加密机制、全部目标接口实测验证
