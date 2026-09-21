# 联通云盘逆向分析文档

本目录包含联通云盘 (com.chinaunicom.bol.cloudapp) APK 的逆向分析文档。

## 文档列表

| 文档 | 说明 |
|------|------|
| [code-structure.md](code-structure.md) | 代码结构分析，包含模块划分、ARouter 路由表、核心类 |
| [api-endpoints.md](api-endpoints.md) | API 接口完整列表，包含服务器地址和所有发现的路径 |
| [target-interfaces.md](target-interfaces.md) | 目标接口详细分析（登录、下载、上传、重命名、移动、空间、创建/删除目录、删除文件） |
| [dispatcher-protocol.md](dispatcher-protocol.md) | **已确认**：dispatcher 单端点协议、签名/加密、操作名清单、各操作参数 |

## 分析状态

### 已完成
- [x] APK 基本信息提取
- [x] 加固方式识别（梆梆加固/Secneo）
- [x] 模块结构分析（基于 ARouter 路由）
- [x] API 服务器地址发现
- [x] API 路径字符串提取
- [x] 第三方库识别
- [x] 目标接口详细分析
- [x] dispatcher 协议确认（签名/加密/操作名）
- [x] 登录、列表、上传、下载、重命名、删除 端到端实测通过（`wocloud_ops.py`）

### 待完成
- [ ] 真实设备 Frida dump（需要绕过反调试）
- [ ] mitmproxy 抓包对照
- [ ] 其余 dispatcher 操作（移动/复制/分享/家庭云）参数确认

## 关键发现

### 服务器地址
- **开发环境**: `https://dev-wocloud.pan.wo.cn:8443`
- **生产 API（dispatcher）**: `https://panservice.mail.wo.cn`
- **生产上传**: `https://du.smartont.net:8443`（`/openapi/client/upload2C`）
- **网页版**: `https://pan.wo.cn`
- **客户端 ID**: App `1001000035`，网页版 `1001000021`

### 主要 API 路径前缀
- `/api-user/` - 用户认证
- `/api/bff/` - BFF 层接口
- `/wohome/` - 云盘核心接口
- `/openapi/` - 文件上传下载
- `/wocloud/` - 云盘功能

### 重要更正（见 dispatcher-protocol.md）
业务操作**不是**每个功能一个 URL，而是统一 POST 到
`/{api-user|wohome}/dispatcher`，用请求头 `key` 传操作名，
body 为 `{param: AES-CBC(参数), clientId, secret}`。

### 目标接口状态

| 接口 | 状态 | 说明 |
|------|------|------|
| 登录 | ✅ 已发现 | QRCode 登录、手机号登录 |
| 下载 | ✅ 已发现 | `/openapi/download` |
| 上传 | ✅ 已发现 | `/openapi/client/upload2C` |
| 文件列表 | ✅ 已确认 | dispatcher 操作名 `QueryAllFiles` |
| 重命名 | ✅ 已确认 | dispatcher 操作名 `RenameFileOrDirectory` |
| 移动 | ✅ 已确认 | dispatcher 操作名 `MoveFile` |
| 创建目录 | ✅ 已确认 | dispatcher 操作名 `CreateDirectory` |
| 删除文件/目录 | ✅ 已确认 | dispatcher 操作名 `DeleteFile` |
| 空间大小 | ✅ 已确认 | 用量在 `AppQueryUser` 返回的 `usageInfo`（allSpace/usedSpace）中 |
| 登录（密码） | ✅ 已实测 | 图形验证码 + `PcWebLogin` → 短信 `PcLoginVerifyCode` → `access_token` |

## 工具脚本

### Python（推荐）

| 脚本 | 说明 |
|------|------|
| `wocloud_ops.py` | 登录 / 列表 / 上传 / 下载 / 重命名 / 删除 全部实现，**已端到端实测通过** |

```bash
pip install pycryptodome
python3 wocloud_ops.py                 # 交互式：图形验证码回车采用 OCR 结果，再输短信码
WO_TOKEN=xxx python3 wocloud_ops.py    # 已有 token 时跳过登录
```

### Frida 脚本

| 脚本 | 说明 |
|------|------|
| `dump_classes.js` | 枚举所有加载的类 |
| `dump_detailed.js` | Dump 类详细信息 |
| `bypass_antidebug.js` | 反调试绕过（未完成） |

### 使用方法

```bash
# 启动 Frida Server
adb root
adb shell "/data/local/tmp/frida-server -D &"

# 附加到应用
frida -U -f com.chinaunicom.bol.cloudapp -l dump_detailed.js

# 或使用 await spawn
frida -U -W com.chinaunicom.bol.cloudapp -l bypass_antidebug.js
```

## 注意事项

1. **加固保护**: 该 APK 使用梆梆加固，静态分析只能看到壳代码
2. **反调试**: 应用有反调试机制，在模拟器上会崩溃
3. **环境检测**: 应用会检测模拟器环境
4. **动态分析**: 需要在真实设备上使用 Frida 进行动态分析

## 文件结构

```
liantong/
├── liantong.apk              # 原始 APK
├── classes.dex               # 提取的 DEX 文件
├── jadx-output/              # Jadx 反编译输出
├── extracted/                # 提取的原生库
├── docs/                     # 分析文档（本目录）
│   ├── README.md
│   ├── dispatcher-protocol.md    # 已确认的协议/操作名/参数/错误码
│   ├── api-endpoints.md
│   ├── target-interfaces.md
│   └── code-structure.md
├── wocloud_ops.py            # Python 操作脚本（已实测）
├── dump_classes.js           # Frida: 枚举类
├── dump_detailed.js          # Frida: 详细信息
└── bypass_antidebug.js       # Frida: 反调试绕过
```

## 下一步

1. 扩展 `wocloud_ops.py`：移动/复制/分享/建目录/回收站
2. 大文件分片上传与秒传（MD5）逻辑
3. 家庭云（`spaceType=1`）与保密空间（`spaceType=4`）分支验证
4. 其余 46 个 dispatcher 操作名的参数确认（见 dispatcher-protocol.md）
