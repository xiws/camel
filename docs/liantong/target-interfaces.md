# 联通云盘目标接口（已确认版）

> **2026-09-20 更新**：本文档原先大量标注「推测 / 待确认」。经 `pan.wo.cn` 网页版
> bundle 逆向 + 真实账号端到端实测，下列目标接口**全部确认**。
>
> 协议层细节（签名、AES、clientId/密钥、错误码、踩坑记录）见
> [dispatcher-protocol.md](dispatcher-protocol.md)；本文档聚焦「每个功能怎么调」。

## 0. 核心结论

**联通云盘没有「一个功能一个 URL」。** 全部业务操作统一 POST 到 dispatcher 端点，
用请求头里的 `key` 分发操作名：

```
POST https://panservice.mail.wo.cn/{api-user|wohome}/dispatcher
header.key = 操作名（如 QueryAllFiles）
```

- `channel = api-user`：账号类（登录、注册、用户信息）
- `channel = wohome`：云盘业务（列表、上传记录、重命名、删除、下载地址…）

这解释了为什么在 `classes.dex` 里搜不到 `/wohome/open/v1/file/rename`、
`/file/delete` 之类的路径——它们并不存在（原文档的「推测路径」已作废）。

唯一不走 dispatcher 的云盘接口是**文件上传**（multipart 直传上传域名）。

## 1. 登录

### 1.1 网页版密码登录（✅ 已实测）

三步，第三步必须人工输入短信验证码：

| 步骤 | 操作名 / 请求 | 说明 |
|---|---|---|
| 1 | `PcWebLogin` (api-user) | 参数 `{phone, password, uuid, verifyCode, clientSecret}`。`clientSecret` 必须填 `queryConfigByClientId(clientId).secretKey` |
| 2 | `GET /api-user/getverifycode?uuid=<uuid>` | 图形验证码图片（60x22 JPEG），`uuid` 为客户端生成的 UUID v4 |
| 3 | `PcLoginVerifyCode` (api-user) | 参数 `{phone, password, uuid, verifyCode, messageCode, clientSecret}`，返回 `DATA.access_token` |

实测响应序列：

```
PcWebLogin(无图形验证码)  -> 6006 图形验证码未输入或缺失参数
PcWebLogin(带图形验证码)  -> 0000 成功  DATA:{needSmsCode:"1"}   ← 此时服务端已发出短信
PcLoginVerifyCode(短信码) -> 0000 成功  DATA:{access_token:...}
```

相关类：`LoginActivity`、`LoginPwdFragment`、`LoginCodeFragment`、`LoginMainViewModel`
（ARouter 路由 `/account/login`）。

### 1.2 其他登录方式（确认存在，未逐一实测）

| 操作名 | 说明 |
|---|---|
| `PcLoginVerifyCodeShow` | 查询是否需要图形验证码 |
| `AppLoginByMobile` | App 手机号登录（字段名被加固清空，未确认） |
| `AppRegisterV2` | 注册 |
| `AppRetrievePasswordV2` / `ResetPwd` / `VerifySetPwd` | 找回/重置密码 |

直连路径：`POST /api-user/sendMessageCodeBase`（发短信，`{func:"pc_send", clientId, param: AES(...)}`）、
`POST /api-user/api/user/ticket`、`/api-user/v1/auth/authentication-generate`。

二维码登录（未实测）：`/wohome/open/v1/QRCode/generate`、`/QRCode/query`、
`/QRCode/login`、`/QRCode/scan`、`/QRCode/cancelLogin`。

## 2. 文件列表（✅ 已实测）

| 项 | 值 |
|---|---|
| 操作名 | `QueryAllFiles`（channel `wohome`） |
| 参数 | `{spaceType, parentDirectoryId, pageNum, pageSize, sortRule}` |
| 家庭云 | 追加 `familyId` |
| 保密空间 | 追加 `psToken` |
| 返回 | `DATA.files[]`，字段 `name` / `size` / `id` / `fid` / `type` / `fileType` / `parentDirectoryId` |

**注意**：`pageNum` 从 **0** 开始（传 1 返回空列表）；列表项字段是 `name`/`size`，
不是 `fileName`/`fileSize`（前端在渲染时才做重命名映射）。

相关类：`AllFileListRepository.kt`、`FileListViewModel.kt`、`FileListSelectionController.kt`。

其他列表类操作：

| 操作名 | 说明 | 参数 |
|---|---|---|
| `QueryDirectorys` | 目录树 | `{spaceType, parentDirectoryId}` |
| `QueryTypeFiles` | 按类型列文件 | 同 QueryAllFiles |
| `GetSearchDirectory` | 搜索路径 | `(params, ["id","dirName"])` |
| `SearchFile` | 搜索 | `{searchType, keyWord, pageNo, pageSize}` |
| `GetDownloadUrlV3` | 下载地址 v3 | `{fidList[], dirList[]}` |

## 3. 上传（✅ 已实测，不走 dispatcher）

```
POST {uploadHost}/openapi/client/upload2C      multipart/form-data
```

- `uploadHost` 由 `GetZoneInfo {appId:"10000001"}` 下发，实测 `https://hyupload.pan.wo.cn`
  （bundle 里写死的 `du.smartont.net:8443` 是备用地址）
- 表单字段：`file`、`uniqueId`(`<毫秒>_<6位随机>`)、`accessToken`、`fileName`、`fileSize`、
  `totalPart`、`partSize`、`partIndex`、`channel="wocloud"`、`directoryId`、`psToken`、
  `fileInfo=AES(JSON, token)`
- **`fileInfo` 解密后必须包含 `batchNo`(32 位随机) 与 `spaceType`**，否则报
  `{"code":"1000","msg":"请求参数错误"}`
- 成功返回 `{"code":"0000","data":{"fid","fileVersion","wcFileId"},"msg":"上传成功"}`

相关类：`UploadManager`、`UploadTask`、`UploadRequest`、`CloudUploadFileRepository.kt`
（Kotlin 协程实现 `upload2C`）、`UploadTempFileRequest.kt`（`/openapi/client/uploadTemp`）。

## 4. 下载（✅ 已实测）

| 步骤 | 操作名 / 请求 | 说明 |
|---|---|---|
| 1 | `GetDownloadUrl` (wohome) | 参数 `{fidList:[fid], clientId, spaceType}`，返回 `DATA[0].downloadUrl` |
| 2 | `GET <downloadUrl>` | 直链形如 `https://hydownload.pan.wo.cn/openapi/download?fid=...`，带 `accesstoken` 头 |

**关键**：`fidList` 必须传列表里的 **`fid`**（长 base64 串），不是 `id`（32 位 hex）——
传 `id` 会得到 `1000 请求参数错误`。

`GetDownloadUrlV2` / `GetDownloadUrlV3` 同样要求传 `fid`；V3 返回 packdown 打包下载链接。

相关类：`DownloadTask`、`DownloadManager`、`DMDownloader`。

## 5. 重命名（✅ 已实测）

| 项 | 值 |
|---|---|
| 操作名 | `RenameFileOrDirectory`（channel `wohome`） |
| 参数 | `{spaceType, type, fileType, id, name}` |
| 取值 | `id` 用列表项的 `id`；`type` 用列表项的 `type`（文件为 1）；`fileType` 用列表项的 `fileType`（txt 为 `"4"`）；`name` 为新文件名 |

实测：`test.txt` → `test2.txt` 返回 `0000 成功`，列表立即生效。

> 原文档推测的 `/wohome/open/v1/file/rename` **不存在**。

## 6. 删除（✅ 已实测）

| 项 | 值 |
|---|---|
| 操作名 | `DeleteFile`（channel `wohome`） |
| 参数 | `{spaceType, vipLevel, dirList[], fileList[]}` |
| 取值 | 文件 ID 放 `fileList`，目录 ID 放 `dirList`（文件与目录共用这一个接口） |

实测删除成功，列表清空。

删除后文件进回收站，相关操作：`QueryRecycleDataNumber`（数量）、
`DeleteRecycleData` / `EmptyRecycleData`（清空回收站，`RecycleBinActivity`）。

> 原文档推测的 `/wohome/open/v1/file/delete`、`/file/dir/delete` **不存在**。

## 7. 其他已确认操作（channel `wohome`）

| 操作名 | 说明 | 参数 |
|---|---|---|
| `CreateDirectory` | 新建目录 | `{isCouldRepeat, spaceType, parentDirectoryId, directoryName}` |
| `MoveFile` | 移动 | `{targetDirId, sourceType, targetType, dirList[], fileList[]}` |
| `MoveInOrOutPrivateSpace` | 保密空间进出 | `{targetDirId, moveType, dirIds[], fileIds[]}` |
| `CopyFile` | 复制 | 同移动结构 |
| `ShareFile` | 创建分享 | `{fileIds[], fileFolderIds, days, autoFill}` |
| `CancelOrDelShareFile` | 取消/删除分享 | `{shareId, operType}` |
| `AllShareFileList` | 分享列表 | — |
| `QueryUploadLog` | 上传记录 | `{clientId, pageNo, pageSize, sortType, orderType}` |
| `DeleteUploadLog` | 清除上传记录 | — |
| `QrySysWcFileBackUp` | 备份查询 | `{pageSize, id, spaceType}` |
| `QueryAlbumBackupV2` / `QueryAlbumBackupDeviceList` | 相册备份 | `{pageSize, id, deviceId}` |
| `QueryWjsyBackup` | 备份查询 | `{pageSize, id}` |
| `QuerySysConfig` | 系统配置 | 无参实测返回 `9999 系统异常`，参数待确认 |
| `QueryMuid` | 设备 MUID | 无参可用，返回 `{muid}` |
| `GetZoneInfo` | 上传域名下发 | `{appId:"10000001"}` |
| `AppQueryUser` | 用户信息（api-user） | `{accessToken}`，DATA 需用 `secretKey` 解密 |
| `AppLogout` | 登出（api-user） | — |

「空间大小」未单列接口：用量信息来自 `AppQueryUser` 返回的用户信息
（`usageInfo.allSpace` / `usedSpace`），前端读取 `vuex.user.usageInfo`。

## 8. 关键参数速查

```
spaceType   0=个人云  1=家庭云  4=保密空间
fileType    0=全部    1=图片    2=视频    3=音频    4=文档    5=其他   (queryFileType)
根目录 ID   "0"
列表分页    pageNum 从 0 开始
上传 uniqueId   <毫秒时间戳>_<6位随机字母数字>
上传 batchNo    32 位随机字母数字（放在加密的 fileInfo 内）
```

## 9. 端到端实测结果（2026-09-20）

| 步骤 | 结果 |
|---|---|
| 登录 | ✅ 图形验证码 → `PcWebLogin` → 短信 → `access_token` |
| 列表 | ✅ `QueryAllFiles` 返回 `0000` |
| 上传 | ✅ `upload2C` 返回 `上传成功` |
| 下载 | ✅ 14 字节，md5 与上传一致 |
| 重命名 | ✅ `test.txt` → `test2.txt` |
| 删除 | ✅ 列表清空 |

验证脚本：`wocloud_ops.py`（仓库根目录）。

## 10. 尚未验证的部分

- `AppLoginByMobile` 的字段名（dex 中该类字段被加固清空）
- `QuerySysConfig` 的必填参数
- 家庭云（`spaceType=1`）与保密空间（`spaceType=4`）分支的 `familyId` / `psToken` 传递
- 大文件分片上传（只验证了单分片的 `totalPart=1`）
- 秒传 / 断点续传逻辑（前端 `computeMD5Success` 会先算 MD5）

## 11. Go 端实现实测（2026-09-21）

用 `camel` CLI 的真实凭证对下面这些操作做了端到端实测（探针目录内完成并已清理）。

| 操作 | dispatcher 操作名 | 结果与参数 |
|---|---|---|
| 新建目录 | `CreateDirectory` | ✅ `{isCouldRepeat:"0", spaceType:"0", parentDirectoryId, directoryName}`，返回 `DATA.id` |
| 重命名 | `RenameFileOrDirectory` | ✅ `{spaceType:"0", type:1, fileType:"4", id, name}` |
| 移动（文件/目录） | `MoveFile` | ✅ `{targetDirId, sourceType:"0", targetType:"0", dirList:[], fileList:[id]}`，目录放 `dirList` |
| 复制 | `CopyFile` | ✅ 参数同移动 |
| 删除（文件/目录） | `DeleteFile` | ✅ 删除目录会递归删除其中内容 |
| 下载 | `GetDownloadUrl` | ✅ `fidList` 要传列表项的 `fid` |
| 新建空文件 | —— | ❌ 无可用手段，见下 |

### 必须知道的四个行为

1. **复制遇同名会自动改名**：目标位置已有同名文件时，`CopyFile` 得到的是 `a(1).txt`，
   既不报错也不覆盖。所以「复制成新名字」需要复制前后对比目录、找出新条目再改名。
2. **上传接口拒绝 0 字节文件**：`upload2C` 对 `fileSize=0` 一律返回 HTTP 400 Bad Request，
   实测四种字段组合（`partSize`/`totalPart` 取 0/1/1024）全部失败；
   声明 `fileSize=1` 而实际传 0 字节则返回 HTTP 500。因此联通无法创建空文件。
3. **`UploadFile` 这个操作名存在**：传 `fileName`/`directoryId`/`fileSize`/`fileType` 的组合
   均返回 `1000 参数错误`，用途未确认。（判别技巧：未知操作名的响应不含 `RSP` 字段。）
4. **删除是异步生效的**：`DeleteFile` 立刻返回 `0000`，但列表要几秒后才更新，
   脚本里「删完立刻列一次」会看到旧结果。
