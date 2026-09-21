# 联通云盘 dispatcher 协议（已确认）

本文档记录对 `pan.wo.cn` 网页版前端 bundle 的逆向确认结果，用于替代
`target-interfaces.md` 中标注为「推测/待确认」的接口。

结论先行：**该 App/网页版没有「每个功能一个 URL」，全部业务操作走同一个 dispatcher 端点，
用请求头里的 `key` 分发操作名。** 这是静态分析在 `classes.dex` 中找不到
`/file/rename`、`/file/delete` 等路径的原因——它们不存在。

## 1. 端点与请求结构

```
POST https://panservice.mail.wo.cn/{channel}/dispatcher
Content-Type: application/json
accesstoken: <登录后的 token，未登录为空串>
```

`channel` 只有两个取值：

- `api-user` —— 账号相关（登录、注册、用户信息、登出、短信/图形验证码）
- `wohome` —— 云盘业务（文件列表、重命名、删除、移动、上传任务、下载地址）

请求体：

```json
{
  "header": {
    "key": "QueryAllFiles",
    "resTime": 1789867404906,
    "reqSeq": 136334,
    "channel": "wohome",
    "sign": "<md5>",
    "version": ""
  },
  "body": {
    "param": "<base64(AES-CBC(JSON.stringify(业务参数)))>",
    "clientId": "1001000021",
    "secret": true
  }
}
```

- `resTime` = `Date.now()`（毫秒）
- `reqSeq` = `Math.floor(89999 * Math.random()) + 100000`
- `sign` = `md5(key + resTime + reqSeq + channel + version + "")`，输出**小写 hex**
- `secret` 为 `true`（api-user 由 js 硬编码，wohome 取决于是否带 clientId 包装）

## 2. 加密参数（实测确认）

```
算法:  AES-128-CBC / PKCS7
key:   secretKey[:16]           // 见下表
iv:    "wNSOYIB1k1DjY5lA"       // window.obfuscatorAESIv (来自 /bridge.js)
输出:  base64                   // 等价 CryptoJS AES.encrypt(...).toString()
```

密钥来源：前端 `queryConfigByClientId(clientId).secretKey`。

| clientId | secretKey | 服务端是否认可 |
|---|---|---|
| `1001000021` | `XFmi9GS2hzk98jGX` | ✅ 实测通过 |
| `1001000001` | (表中为混淆值) | ❌ 返回 9002 |
| `1001000035` / `1001001046` / `1001000015` / `1001000017` / `1001000024` | 表中对应 16 字符串 | ❌ 返回 9002 |

> `XFmi9GS2hzk98jGX` 同时出现在 `/bridge.js` 的 `window.obfuscatorAESCodeProd`。
> 前端 bundle 中 `clientId → secretKey` 的表带大量混淆诱饵，实测只有 `1001000021` 可用。

## 3. 响应格式

```json
{"STATUS": "200", "MSG": "服务调用成功！", "LOGID": "...",
 "RSP": {"RSP_CODE": "0000", "RSP_DESC": "成功", "DATA": {...}}}
```

- `RSP_CODE == "0000"` 为成功
- `DATA` 在 `channel=api-user` 时为明文 JSON；`wohome` 的响应解密取决于服务端返回的
  `body.secret` / `body.key` 标记
- 实测错误码：
  - `9002` 解密失败（密钥错）
  - `1000` 请求参数错误（缺必填字段）
  - `8001` 账号未注册或密码错误

## 4. 操作名清单

### channel = api-user

| 操作名 | 说明 |
|---|---|
| `PcWebLogin` | 网页版密码登录 |
| `PcLoginVerifyCode` | 短信二次验证（返回 access_token） |
| `PcWebLoginVerifyCodeShow` | 图形验证码开关查询 |
| `AppLoginByMobile` | App 手机号登录 |
| `AppRegisterV2` | 注册 |
| `AppQueryUser` | 查询用户信息 |
| `AppUpdateUserV2` | 更新用户信息 |
| `AppLogout` | 登出 |
| `AppRetrievePasswordV2` | 找回密码 |
| `ReSetPwd` / `VerifySetPwd` / `GenerateReSetPwdSignCode` | 重置密码相关 |
| `QueryFamilyGroups` | 家庭组列表 |

其他 `api-user` 直连路径（非 dispatcher）：

- `GET  /api-user/getverifycode?uuid=...` 图形验证码图片
- `POST /api-user/sendMessageCodeBase` 发送短信验证码
- `POST /api-user/api/user/ticket`、`/api-user/v1/auth/authentication-generate` 等

### channel = wohome

| 操作名 | 说明 | 已确认参数 |
|---|---|---|
| `QueryAllFiles` | 文件列表 | `{spaceType, parentDirectoryId, pageNum, pageSize, sortRule}`；家庭云追加 `familyId`，保密空间追加 `psToken`。**实测：`pageNum` 从 0 开始（传 1 返回空）；返回项字段是 `name`/`size`/`id`/`fid`/`type`/`fileType`** |
| `QueryDirectorys` | 目录树 | `{spaceType, parentDirectoryId}`（家庭云带 `familyId`，保密空间带 `psToken`） |
| `QueryTypeFiles` | 按类型列文件 | 同上列表参数 |
| `GetSearchDirectory` | 搜索路径 | `(params, ["id","dirName"])` |
| `SearchFile` | 搜索 | `{searchType, keyWord, pageNo, pageSize}`，`searchType` 个人云 `"2"` / 家庭云 `"1"` |
| `RenameFileOrDirectory` | 重命名 | `{spaceType, type, fileType, id, name}` |
| `CreateDirectory` | 新建目录 | `{spaceType, name/parentDirectoryId, ...}` |
| `DeleteFile` | 删除（文件/目录） | `{spaceType, vipLevel, dirList[], fileList[]}` |
| `MoveFile` | 移动 | `{targetDirId, sourceType, targetType, dirList[], fileList[]}` |
| `MoveInOrOutPrivateSpace` | 保密空间进出 | `{targetDirId, moveType, dirIds[], fileIds[]}` |
| `CopyFile` | 复制 | 同移动结构 |
| `GetDownloadUrl` | 取下载地址 | `{fidList[], clientId, spaceType}` → `DATA[0].downloadUrl`。**实测：fidList 必须传 `fid`（不是列表里的 `id`），返回 hydownload.pan.wo.cn 直链** |
| `GetDownloadUrlV2` / `GetDownloadUrlV3` | 取下载地址 v2/v3 | `{fidList[], dirList[]}`，同样要传 `fid`；V3 返回 `{downloadUrl}`（packdown 打包下载）|
| `ShareFile` | 创建分享 | `{fileIds[], fileFolderIds, days, autoFill}` |
| `CancelOrDelShareFile` | 取消/删除分享 | — |
| `AllShareFileList` | 分享列表 | — |
| `QueryUploadLog` | 上传记录 | `{clientId, pageNo, pageSize, sortType, orderType}` |
| `DeleteUploadLog` | 删除上传记录 | — |
| `GetZoneInfo` | 获取上传域名 | `{appId: "10000001"}` → `DATA.url`（实测为 `https://hyupload.pan.wo.cn`） |
| `QuerySysConfig` / `QueryMuid` | 系统配置 / MUID | `QuerySysConfig` 无参实测返回 9999，参数待确认 |
| `QrySysWcFileBackUp` | 备份查询 | `{pageSize, id, spaceType}` |
| `QueryAlbumBackupV2` / `QueryAlbumBackupDeviceList` | 相册备份 | `{pageSize, id, deviceId}` |

`spaceType` 取值：`0` 个人云、`1` 家庭云、`4` 保密空间。

## 5. 上传接口（非 dispatcher）

```
POST {uploadHost}/openapi/client/upload2C
Content-Type: multipart/form-data
```

`uploadHost` 由 dispatcher 的 `GetZoneInfo {appId:"10000001"}` 下发，实测为
`https://hyupload.pan.wo.cn`。

表单字段（来自前端 uploader 的 `processParams`）：

| 字段 | 说明 |
|---|---|
| `file` | 分片二进制（`fileParameterName`，默认 `file`） |
| `uniqueId` | 文件名唯一标识，格式 `<毫秒时间戳>_<6位随机字符>`（必填） |
| `accessToken` | 登录 token |
| `fileName` | 文件名 |
| `fileSize` / `totalPart` / `partSize` / `partIndex` | 分片信息（小文件 totalPart=1, partIndex=1） |
| `channel` | 固定 `wocloud` |
| `directoryId` | 目标目录 ID（根目录为 `"0"`） |
| `psToken` | 保密空间 token（可空串） |
| `fileInfo` | `AES(JSON(...), token)`，**解密后必须含 `batchNo`(32位随机) 与 `spaceType`** |

失败排查记录（很重要）：

- 缺 `uniqueId` → HTTP 400 Bad Request
- `fileInfo` 少 `batchNo` / `spaceType` → `{"code":"1000","msg":"请求参数错误"}`
- `fileInfo` 用错密钥（如 secretKey 而非 token）→ HTTP 500
- 成功后返回 `{"code":"0000","data":{"fid","fileVersion","wcFileId"},"msg":"上传成功"}`

## 6. 登录流程（实测）

网页版密码登录是**两步**，且带风控：

1. `PcWebLogin {phone, password, uuid, verifyCode, clientSecret}`
   - 首次成功响应 `8001`（凭据不匹配）或 `6006`（要求图形验证码）
   - 返回 `6008` 表示需要短信二次验证
2. 图形验证码：`GET /api-user/getverifycode?uuid=<uuid>` 拿图片，将人工识别的 4 位码
   作为 `verifyCode`（`uuid` 用同一个 UUID v4）
3. 短信验证码：`PcLoginVerifyCode {phone, password, uuid, verifyCode, messageCode, clientSecret}`
   - 返回 `DATA.access_token`，即后续请求的 `accesstoken`

关键点：`clientSecret` 字段必须填 `queryConfigByClientId(clientId).secretKey`，
填空串会得到 `1000 参数错误`。

## 7. 实测记录（2026-09-20）

全链路已在真实账号上跑通，产物脚本 `wocloud_ops.py`：

| 步骤 | 操作 | 结果 |
|---|---|---|
| 1 | 图形验证码 OCR → `PcWebLogin` → 短信 `PcLoginVerifyCode` | ✅ 拿到 `access_token`（证明密码 `Jx960721!` 正确）|
| 2 | `QueryAllFiles` | ✅ 返回文件列表 |
| 3 | `upload2C` | ✅ `上传成功` |
| 4 | `GetDownloadUrl` + GET | ✅ 14 字节，内容与上传 md5 一致 |
| 5 | `RenameFileOrDirectory` | ✅ `test.txt` → `test2.txt` |
| 6 | `DeleteFile` | ✅ 列表清空 |

排查过程中的关键错误码：`9002` 密钥错、`1000` 参数错/缺必填、`8001` 凭据错、
`6006` 需图形验证码、`6007` 图形验证码错、`9999` 系统异常（参数不对）。

## 8. 与旧文档的差异

- 旧文档中的 `/wohome/open/v1/file/delete`、`/file/move`、`/file/rename` 等路径
  **不存在**，实际为 dispatcher 操作名（见第 4 节）。
- 服务器域名：文档里的 `dev-wocloud.pan.wo.cn` 为测试环境；
  生产 API 是 `panservice.mail.wo.cn`，上传是 `du.smartont.net:8443`。
- 请求确实带签名 + AES 加密体，不是明文 JSON。
