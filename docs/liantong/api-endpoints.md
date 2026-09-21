# 联通云盘 API 接口分析

> **2026-09-20 已确认并实测**。关键更新：
> - 业务接口**不是**一功能一 URL，而是统一 POST 到 `/{api-user|wohome}/dispatcher`，用请求头 `key` 传操作名；
>   下面列出的很多「路径」实际是 dispatcher 的操作名或直连子路径。
> - 请求带**签名 + AES 加密体**（`sign = md5(key+resTime+reqSeq+channel+version)`，body 为 `{param: AES(...)}`）。
> - 生产域名是 `panservice.mail.wo.cn`（API）、`hyupload.pan.wo.cn`（上传）、`hydownload.pan.wo.cn`（下载），
>   `dev-wocloud.pan.wo.cn` / `tj*.pan.wo.cn` 是旧环境或备用地址。
>
> 协议细节见 [dispatcher-protocol.md](dispatcher-protocol.md)，各功能调法见 [target-interfaces.md](target-interfaces.md)。

## 核心协议（dispatcher）

```
POST https://panservice.mail.wo.cn/{api-user|wohome}/dispatcher
Content-Type: application/json
accesstoken: <登录后的 access_token>

{
  "header": {"key": "QueryAllFiles", "resTime": <ms>, "reqSeq": <rand>,
             "channel": "wohome", "sign": "<md5>", "version": ""},
  "body":   api-user -> {"param": AES(参数, SECRET_KEY), "clientId": "1001000021", "secret": true}
            wohome   -> {"param": AES(参数+clientId, TOKEN), "key": true}
}
```

- AES-128-CBC / PKCS7，密钥取前 16 字节，IV = `wNSOYIB1k1DjY5lA`
- 实测可用 clientId：`1001000021`，对应 secretKey `XFmi9GS2hzk98jGX`
- `wohome` 的响应 DATA 需用 TOKEN 解密，`api-user` 为明文
- 已确认的 dispatcher 操作名共 46 个，完整清单见 dispatcher-protocol.md 第 4 节

## 服务器地址

### 生产环境（实测可用）
- **API（dispatcher）**: `https://panservice.mail.wo.cn`
- **上传**: `https://hyupload.pan.wo.cn`（由 `GetZoneInfo` 下发）
- **下载**: `https://hydownload.pan.wo.cn`
- **网页版**: `https://pan.wo.cn`（前端 bundle 含 `/bridge.js`、内联 webpack runtime）

### 旧环境 / 备用
- **开发环境**: `https://dev-wocloud.pan.wo.cn:8443`
- **备用上传**: `https://du.smartont.net:8443`（旧硬编码值）
- **联通门户**: `https://nisportal.10010.com:9001`

### 客户端标识
- **网页版 clientId**: `1001000021`（默认）
- **App clientId**: `1001000035`
- **已确认密钥**: `1001000021` → `XFmi9GS2hzk98jGX`

## API 路径分类

> 以下路径来自 `classes.dex` 字符串池。需注意：
> - `/wohome/...`、`/wocloud/...` 中的很多「路径」实际是 **dispatcher 的操作名**（如
>   `QuickShare` 类子路径）或直连子路径；文件列表/重命名/删除/移动等**没有独立 URL**，
>   全部走 `POST /wohome/dispatcher` + `header.key`。
> - `/api-user/...` 多为真实直连路径（发短信、图形验证码、ticket 等）。
> - `/api/bff/...`、`/openapi/...` 为真实路径。

### 1. 用户认证相关 (/api-user)

| 路径 | 方法 | 说明 |
|------|------|------|
| `/api-user/api/user/ticket` | POST | 获取用户票据 |
| `/api-user/dispatcher` | POST | 调度接口 |
| `/api-user/getverifycode` | POST | 获取验证码 |
| `/api-user/sendMessageCodeBase` | POST | 发送短信验证码 |
| `/api-user/upload.do` | POST | 用户上传 |

### 2. BFF 层接口 (/api/bff)

| 路径 | 方法 | 说明 |
|------|------|------|
| `/api/bff/homePage` | GET | 首页数据 |
| `/api/bff/app/banner` | GET | 应用横幅广告 |
| `/api/bff/app/home/toolboxs` | GET | 首页工具箱 |
| `/api/bff/app/customTools/save` | POST | 保存自定义工具 |
| `/api/bff/app/toolsCornerMark/save` | POST | 保存工具角标 |
| `/api/bff/ai/resource` | GET | AI 资源 |
| `/api/bff/home/school/v2` | GET | 校园版首页 V2 |
| `/api/bff/notify/status/change` | POST | 通知状态变更 |
| `/api/bff/discovery/page/tab/list` | GET | 发现页标签列表 |
| `/api/bff/gtns/submit/device/token` | POST | 提交设备 Token |
| `/api/bff/resource/query/popupAndFlow` | GET | 查询弹窗和流量 |
| `/api/bff/user/rebind/userRebindLogin` | POST | 用户重绑登录 |
| `/api/bff/yunPhone/password/free/login` | POST | 云手机免密登录 |

### 3. 云盘核心接口 (/wohome)

#### 3.1 文件操作 (/wohome/open)

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/open/v1/file/path` | GET/POST | 获取文件路径 |
| `/wohome/open/file/getMultipleDownloadUrls` | GET | 获取批量下载链接 |
| `/wohome/open/im/file/store/copy` | POST | IM 文件存储复制 |
| `/wohome/open/im/file/store/status` | GET | IM 文件存储状态 |
| `/wohome/open/v1/quickShare/batchSave` | POST | 批量保存快传文件 |
| `/wohome/open/v1/quickShare/batchDelete` | POST | 批量删除快传文件 |
| `/wohome/open/v1/quickShare/queryPage` | GET | 分页查询快传 |
| `/wohome/open/v1/quickShare/querySystemDir` | GET | 查询系统目录 |
| `/wohome/open/v1/resource/query/resources` | GET | 查询资源 |
| `/wohome/open/v1/resource/query/home-resource/app` | GET | 查询首页应用资源 |
| `/wohome/open/v1/resource/query/open-screen/app` | GET | 查询开屏应用 |
| `/wohome/open/v1/deleteDynNews` | POST | 删除动态新闻 |
| `/wohome/open/v1/saveUserRedDotStatus` | POST | 保存用户红点状态 |
| `/wohome/open/v1/queryUserRedDotStatus` | GET | 查询用户红点状态 |
| `/wohome/open/v2/intelligentSearch` | POST | 智能搜索 V2 |
| `/wohome/open/v2/prefetch-data` | GET | 预取数据 |
| `/wohome/open/v3/home/search` | GET | 首页搜索 V3 |
| `/wohome/open/v3/home/tag` | GET | 首页标签 V3 |

#### 3.2 登录认证

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/open/v1/QRCode/login` | POST | 二维码登录 |
| `/wohome/open/v1/QRCode/scan` | POST | 二维码扫描 |
| `/wohome/open/v1/QRCode/cancelLogin` | POST | 取消二维码登录 |

#### 3.3 任务管理 (/wohome/pass)

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/pass/file/batch/createTask` | POST | 批量创建文件任务 |
| `/wohome/pass/task/batch/taskStatus` | GET | 批量查询任务状态 |
| `/wohome/pass/task/queryAllFilesNumber` | GET | 查询所有文件数量 |
| `/wohome/pass/task/queryAllDocsNumber` | GET | 查询所有文档数量 |
| `/wohome/pass/task/queryTypeFilesNumber` | GET | 按类型查询文件数量 |
| `/wohome/pass/task/queryBackUpFileNumber` | GET | 查询备份文件数量 |
| `/wohome/pass/task/queryRecycleDataNumber` | GET | 查询回收站数据数量 |
| `/wohome/pass/task/favoriteNumber` | GET | 查询收藏数量 |
| `/wohome/pass/task/queryAlbumBackupV2FilesNumber` | GET | 查询相册备份 V2 文件数 |
| `/wohome/pass/task/queryVideoAndAudioFilesNumber` | GET | 查询视频音频文件数 |

#### 3.4 收藏功能

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wocloud/favorite/addAndCancelFavorite` | POST | 添加/取消收藏 |
| `/wocloud/favorite/list` | GET | 收藏列表 |

#### 3.5 备份功能 (/wohome/backup)

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/backup/queryBackUpFile` | GET | 查询备份文件 |
| `/wohome/backup/queryBackUpFileCount` | GET | 查询备份文件数量 |

#### 3.6 通讯录备份

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/open/v1/contactBackup/applyCert` | POST | 申请证书 |
| `/wohome/open/v1/contactBackup/getKeyAttribute` | GET | 获取密钥属性 |
| `/wohome/open/v1/contactBackup/saveContactBackup` | POST | 保存通讯录备份 |
| `/wohome/open/v1/contactBackup/getContactBackupInfo` | GET | 获取备份信息 |
| `/wohome/open/v1/contactBackup/getContactBackupDetail` | GET | 获取备份详情 |
| `/wohome/open/v1/contactBackup/getContactBackupRecord` | GET | 获取备份记录 |
| `/wohome/open/v1/contactBackup/deleteContactBackupRecord` | POST | 删除备份记录 |
| `/wohome/open/v1/contactBackup/applyEncryptionBusinessKey` | POST | 申请加密业务密钥 |

#### 3.7 通话备份

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/open/v1/callBackup/saveCallBackup` | POST | 保存通话备份 |
| `/wohome/open/v1/callBackup/getCallBackupInfo` | GET | 获取通话备份信息 |
| `/wohome/open/v1/callBackup/getCallBackupDetail` | GET | 获取通话备份详情 |
| `/wohome/open/v1/callBackup/getCallBackupMaxCallId` | GET | 获取最大通话 ID |

#### 3.8 短信备份

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/open/v1/messageBackup/getSmsBackupNo` | GET | 获取短信备份编号 |
| `/wohome/open/v1/messageBackup/saveSmsBackupService` | POST | 保存短信备份服务 |
| `/wohome/open/v1/messageBackup/getLastSmsBackupCount` | GET | 获取上次备份数量 |
| `/wohome/open/v1/messageBackup/getSmsBackupMaxSmsId` | GET | 获取最大短信 ID |
| `/wohome/open/v1/messageBackup/getSmsBackupCountByDevice` | GET | 按设备获取备份数量 |

#### 3.9 知识库

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/knowledge/queryTypeFileList` | GET | 按类型查询文件列表 |

#### 3.10 免费接口 (/wohome/free)

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/free/v1/common/xzqh/list` | GET | 行政区划列表 |
| `/wohome/free/v1/findClassifyRule` | GET | 查找分类规则 |
| `/wohome/free/v1/ai/recognition` | POST | AI 识别 |
| `/wohome/free/v1/webOffice/getFileInfo` | GET | WebOffice 文件信息 |

#### 3.11 媒体相关

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/media/audioDisplay` | GET | 音频展示 |
| `/wohome/open/v1/audio/tag` | GET | 音频标签 |
| `/wohome/open/v1/video/tag` | GET | 视频标签 |
| `/wohome/open/v1/video/progress/get` | GET | 获取视频进度 |
| `/wohome/open/v1/video/progress/save` | POST | 保存视频进度 |
| `/wohome/open/v1/mango/listVideoAll` | GET | 芒果 TV 视频列表 |
| `/wohome/open/v1/mango/listCollectionById` | GET | 按 ID 查询合集 |

#### 3.12 AI 功能

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/open/v1/ai/getLanguage` | GET | 获取 AI 语言 |
| `/wohome/ai/agnet/task/check/duration` | GET | 检查 AI 任务时长 |

#### 3.13 其他

| 路径 | 方法 | 说明 |
|------|------|------|
| `/wohome/dispatcher` | POST | 调度器 |
| `/wohome/open/kanjia/queryStatus` | GET | 看家状态查询 |
| `/wohome/open/v1/oa/pem` | GET | OA PEM 证书 |
| `/wohome/open/v1/clusters/getTargetFreeTraffic` | GET | 获取目标免费流量 |
| `/wohome/share/manage/query` | GET | 分享管理查询 |
| `/wohome/open/v1/album/getPhoneModel` | GET | 获取手机型号 |

### 4. 文件上传接口 (/openapi/client)

**注意**：实际请求发往上传域名（`GetZoneInfo` 下发，实测 `https://hyupload.pan.wo.cn`），
不是 `panservice.mail.wo.cn`。

| 路径 | 方法 | 说明 |
|------|------|------|
| `/openapi/client/upload2C` | POST | 2C 端文件上传（multipart，字段见 target-interfaces.md 第 3 节）|
| `/openapi/client/uploadTemp` | POST | 临时文件上传 |
| `/openapi/client/uploadOCR` | POST | OCR 图片上传 |
| `/openapi/client/uploadOCRTxt` | POST | OCR 文本上传 |

### 5. 文件下载接口 (/openapi)

下载域名实测为 `https://hydownload.pan.wo.cn`（由 GetDownloadUrl 系列接口下发），
路径形如 `/openapi/download?fid=<urlencoded fid>`。

| 路径 | 方法 | 说明 |
|------|------|------|
| `/openapi/download` | GET | 单文件下载（需带 `accesstoken` 头）|
| `/openapi/packdown` | GET | 打包下载（GetDownloadUrlV3 返回）|
| `/openapi/thumbnails` | GET | 缩略图 |

### 6. 文件处理接口

| 路径 | 方法 | 说明 |
|------|------|------|
| `/openapi/file/unzip` | POST | 文件解压 |
| `/openapi/file/getUnzipTask` | GET | 获取解压任务 |

### 7. 其他接口

| 路径 | 方法 | 说明 |
|------|------|------|
| `/api/update/native/fetch` | GET | 获取原生更新 |
| `/api/update/native/report` | POST | 上报更新状态 |
| `/api/crashsdk/validate` | POST | 崩溃 SDK 验证 |
| `/api/v1/crashtrack/upload` | POST | 崩溃日志上传 |

## 请求参数常见字段

基于字符串分析 + 实测确认，API 请求中的常见参数：

```
- clientid: 客户端 ID (1001000035)
- token: 用户令牌
- fileName: 文件名
- filePath: 文件路径
- fileSize: 文件大小
- fileType: 文件类型
- fileCount: 文件数量
- deleteNo / deleteNos: 删除编号
- direction: 方向
- spaceType: 空间类型
- loginType: 登录类型
- autoLogin: 自动登录
- dataSpace: 数据空间
- storeDir: 存储目录
- subdirPath: 子目录路径
- layoutDir: 布局目录
```

**实测确认的关键字段**：

```
spaceType          0=个人云 1=家庭云 4=保密空间
parentDirectoryId  父目录 ID（根目录 "0"）
pageNum / pageSize 分页，注意 pageNum 从 0 开始
sortRule           排序规则
id / fid           列表项两个 ID：id 用于重命名/删除，fid 用于取下载地址
type / fileType    列表项类型（type=1 文件；fileType 0~5 为文件分类）
uuid               图形验证码会话 ID（UUID v4）
verifyCode         图形验证码
messageCode        短信验证码
clientSecret       = queryConfigByClientId(clientId).secretKey，登录接口必填
batchNo            上传批次号（32 位随机，上传必填，在加密的 fileInfo 内）
psToken            保密空间令牌
familyId           家庭云 ID
```

## 响应格式

```json
{"STATUS": "200", "MSG": "服务调用成功！", "LOGID": "...",
 "RSP": {"RSP_CODE": "0000", "RSP_DESC": "成功", "DATA": {...}}}
```

- `RSP_CODE == "0000"` 为成功
- 实测错误码：
  - `9002` 解密失败（密钥/clientId 不对）
  - `1000` 请求参数错误（缺必填字段）
  - `8001` 账号未注册或密码错误
  - `6006` 需要图形验证码 / `6007` 图形验证码错误
  - `6008` 需要短信二次验证
  - `9999` 系统异常（参数不匹配）

上传接口返回的是另一种结构：`{"code":"0000","data":{...},"msg":"上传成功"}`。

## 注意事项

1. 所有 dispatcher 请求都需要 `header.sign` 与 AES 加密体，不是明文 JSON
2. 上传是 multipart 直传上传域名，不经过 dispatcher；`fileInfo` 内必须含 `batchNo` 与 `spaceType`
3. 下载地址接口的 `fidList` 要传 `fid`（不是 `id`）
4. 列表接口 `pageNum` 从 0 开始
5. 登录链路需要图形验证码 + 短信验证码，无法全自动
6. 文件删除先进回收站，`RecycleBinActivity` 可恢复或彻底删除
