# 百度网盘 Android APK 逆向 - API 接口文档

> 来源: `baidu/baidu.apk` (~375MB) 经 jadx 反编译分析
> 反编译输出: `docs/sources/` (105,204 Java 文件)
> APK 版本: 9.15.1.32 (versionCode 250)

---

BDUSS:9kVjUyRmVaTEJpb0lSSGUwOFlVdk51VExMeHdCNXVFQXpKZVNoVDFHMXJQNlZwRVFBQUFBJCQAAAAAAAAAAAEAAADZ4Z4vemg5NjA3MjEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAGuyfWlrsn1pUk

## 1. 认证体系 (Authentication)

### 1.1 认证方式

百度网盘使用 **BDUSS Cookie** 认证，所有 API 请求通过 Cookie 头传递身份信息。

| 凭证 | 说明 | 存储位置 |
|------|------|----------|
| **BDUSS** | 主认证令牌，等同于 session token | `AccountUtils.mBduss` |
| **STOKEN** | 辅助令牌，用于安全验证 | `AccountUtils.stoken` |
| **pToken** | 持久化令牌 | `AccountUtils.pToken` |
| **auth** | 认证标记 | `AccountUtils.mAuth` |
| **bdstoken** | 请求签名，= `MD5(BDUSS).toLowerCase()` | `BaseApi.bdstoken` |

### 1.2 bdstoken 计算

**所有 pan.baidu.com API 请求必须携带 bdstoken 查询参数。**

```java
// BaseApi.java:44
bdstoken = MD5Util.createMD5WithHex(bduss, false);  // BDUSS 的小写 MD5
```

`BaseApi.handlerParams()` (lines 48-61) 会自动将 `bdstoken` 注入到所有请求参数中。

**关键文件**: `com/baidu/netdisk/network/BaseApi.java`

### 1.3 Cookie 构造

```java
// CookieUtils.getCookieByBduss(bduss)
// 完整 Cookie 格式:
Cookie: BDUSS=<bduss>; STOKEN=<stoken>; panPSC=<pan_psc>; ndut_fmt=<fmt>; ndFTID=<ftid>
```

**关键文件**:
- `com/baidu/netdisk/util/CookieUtils.java` — Cookie 构造
- `com/baidu/netdisk/account/AccountUtils.java` — 账号信息管理
- `com/baidu/netdisk/base/network/StokenManager.java` — STOKEN 管理

### 1.4 请求认证头示例

```
Cookie: BDUSS=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx; STOKEN=xxxx; panPSC=xxxx
```

### 1.5 签名算法总结

| API 类型 | 签名方式 | 说明 |
|---------|---------|------|
| 文件列表/重命名/移动/复制/删除/配额 | **无需 sign** | 仅需 BDUSS Cookie + bdstoken 参数 |
| `task/create` | HMAC-SHA1 | `HMAC-SHA1(MD5(content).lower + clientType + channel + DEVUID + timestamp)` |
| 分享 API | HMAC-SHA1 | `HMAC-SHA1(shareid_uk_DEVUID_timestamp)` |
| 我的分享 | MD5 | `MD5(shareId + "_sharesurlinfo!@#")` |

**关键文件**:
- `com/baidu/netdisk/utils/encode/SHA1Util.java` — HMAC-SHA1 实现
- `com/baidu/netdisk/utils/encode/MD5Util.java` — MD5 工具
- `com/baidu/netdisk/cloudfile/io/CloudFileApi.java:167-188` — `makeContent()` 方法

---

## 2. 登录接口 (Login)

### 2.1 登录域名

| 域名 | 用途 | 代码来源 |
|------|------|----------|
| `https://passport.baidu.com` | 主登录 API | `SapiHost.DOMAIN_ONLINE_PASSPORT_URL` |
| `https://wappass.baidu.com` | WAP/Web 登录流程 | `SapiHost.DOMAIN_ONLINE_WAPPASS_URL` |
| `https://wappass.bdimg.com` | 配置 HTTPS | `SapiHost.DOMAIN_ONLINE_CONFIG_HTTPS_URL` |
| `https://openapi.baidu.com` | OAuth/设备 API | `SapiHost.DOMAIN_ONLINE_DEVICE_URL` |

**关键文件**: `com/baidu/sapi2/utils/SapiHost.java`

### 2.2 登录端点一览

| 方式 | 端点 | 方法 | 常量名 |
|------|------|------|--------|
| 用户名/密码 | `/v2/sapi/login` | POST | `SapiEnv.LOGIN_URI` |
| 获取扫码二维码 | `/v2/api/getqrcode` | GET | `SapiEnv.GET_QR_CODE_IMAGE_URI` |
| 轮询扫码结果 | `/v2/api/bdusslogin` | GET | `SapiEnv.GET_QR_LOGIN_RESULT` |
| 扫码加入登录 | `/v3/login/main/qrbdusslogin` | GET | `SapiEnv.GET_QR_JOIN_LOGIN_RESULT` |
| QR App 登录 | `/v2/sapi/qrlogin` | POST | `SapiEnv.QR_APP_LOGIN_URI` |
| 扫码状态检查 | `/channel/unicast` | GET(长轮询) | `SapiEnv.GET_QR_LOGIN_STATUS_CHECK` |
| 一键登录 | `/v3/login/onekeylogin` | POST | `SapiEnv.LOAD_ONE_KEY_LOGIN` |
| 海外一键登录 | `/v3/login/overseaonekeylogin` | POST | `SapiEnv.LOAD_OVERSEA_ONE_KEY_LOGIN` |
| EAC 码换 BDUSS | `/v3/api/login/eacexchangebduss` | POST | `SapiEnv.EAC_CODE_LOGIN` |
| 获取 STOKEN | `/v3/login/api/auth/` | POST | `SapiEnv.GET_STOKEN_URI` |
| 获取 Open BDUSS | `/v3/login/api/authopenbduss` | GET | `SapiEnv.GET_OPEN_BDUSS` |
| 登出 | `/v3/api/user/logout` | POST | `SapiEnv.USER_LOGOUT` |
| 静默 OAuth | `/v3/api/oauth/silentauthorize` | GET | `SapiEnv.SILENT_AUTH` |
| 切换账号 | `/v6/changeAccount` | POST | `SapiEnv.SWITCH_ACCOUNT` |
| SSO 登录 | `/phoenix/account/ssologin` | POST | `SapiEnv.SSO_START_URI` / `SSO_FINISH_URI` |
| 第三方登录 | `/phoenix/account/startlogin` | POST | `SapiEnv.SOCIAL_START_URI` |
| 第三方绑定 | `/phoenix/account/finishbind` | POST | `SapiEnv.SOCIAL_FINISH_AUTH_URI` |
| 海外 SSO | `/phoenix/account/overseasssologin` | POST | `SapiEnv.OVERSEAS_SSO_URI` |
| SDK 配置 | `/v8/sdkconfig/init` | GET | `SapiEnv.SAPI_CONFIG_HTTPS_URI` |
| 登录配置 | `/v8/sdkconfig/loginconfig` | GET | `SapiEnv.SAPI_LOGIN_CONFIG_HTTPS_URI` |
| BDUSS 换 AccessToken | `/v2/sapi/bdussexchangeaccesstoken` | POST | `SapiEnv.OAUTH_URI` |
| 获取用户信息 | `/v2/sapi/center/getuinfo` | GET | `SapiEnv.GET_USER_INFO_URI` |
| 验证码初始化 | `https://passport.baidu.com/cap/init` | POST | `ValidationManager.URL_CAP_INIT` |

**关键文件**: `com/baidu/sapi2/utils/SapiEnv.java`

### 2.3 用户名/密码登录 (`/v2/sapi/login`)

#### 请求参数

| 参数 | 常量名 | 说明 |
|------|--------|------|
| `username` | `REQUEST_KEY_USER_NAME` | 手机号/邮箱 |
| `password` | `REQUEST_KEY_PASSWORD` | RSA 加密后的密码 |
| `isphone` | `REQUEST_KEY_LOGIN_IS_PHONE` | `"0"` 或 `"1"` |
| `isEncrypted` | `REQUEST_KEY_IS_ENCRYPTED` | `"1"` 表示密码已 RSA 加密 |
| `encryptedId` | `REQUEST_KEY_ENCRYPTED_ID` | 加密密钥 ID |
| `encryptedType` | `REQUEST_KEY_ENCRYPTED_TYPE` | 加密类型 |
| `alg` | `REQUEST_KEY_ALG` | 算法标识 |
| `countrycode` | `REQUEST_KEY_COUNTRY_CODE` | 国际区号 |
| `loginmerge` | `REQUEST_KEY_LOGIN_MERGE` | 登录合并标记 (`"1"`) |
| `logLoginType` | `REQUEST_KEY_LOGIN_TYPE` | 登录类型标识 |
| `t` | `REQUEST_KEY_T` | 时间戳 |
| `time` | `REQUEST_KEY_TIME` | 时间 |
| `uid` | `REQUEST_KEY_UID` | 用户 ID |
| `cv` | `REQUEST_KEY_CV` | 客户端版本 |
| `lang` | `REQUEST_KEY_LANG` | 语言 |
| `adapter` | `REQUEST_ADAPTER` | 适配器类型 (`"3"` 或 `"8"`) |
| `session_id` | `REQUEST_KEY_SESSION_ID` | 会话 ID |
| `scanface` | `REQUEST_KEY_SCAN_FACE` | 人脸扫描标记 |
| `supFaceLogin` | `REQUEST_KEY_SUP_FACE_LOGIN` | 支持人脸登录 |
| `liveAbility` | `REQUEST_KEY_LIVE_ABILITY` | 活体检测能力 |
| `enableExternalWeb` | `REQUEST_KEY_ENABLE_EXTERNAL_WEB` | 外部 Web 标记 |

#### 验证码挑战参数 (首次登录被拒后附带)

| 参数 | 常量名 | 说明 |
|------|--------|------|
| `as` | `VALIDATE_KEY_AS` | 验证码答案 |
| `ds` | `VALIDATE_KEY_DS` | 验证码数据源 |
| `tk` | `VALIDATE_KEY_TK` | 验证码 token |

#### 调用方式

```java
// NewLoginAccountAndPassWordTypeBaseView.java:798
SapiAccountManager.getInstance().getAccountService()
    .accountAndPwdLogin(username, password, encryptedUid, callback, extraParams);
```

**关键文件**: `com/baidu/sapi2/result/AccountAndPwdLoginResult.java`

### 2.4 短信验证码登录

| 参数 | 常量名 | 说明 |
|------|--------|------|
| `username` | `REQUEST_KEY_USER_NAME` | 手机号 |
| `sms` | `REQUEST_KEY_SMS` | 短信标记 |
| `smsvc` | `REQUEST_KEY_SMS_VC` | 短信验证码 |
| `countrycode` | `REQUEST_KEY_COUNTRY_CODE` | 国际区号 |
| `encryptedId` | `REQUEST_KEY_ENCRYPTED_ID` | 加密密钥 ID |
| `encryptedType` | `REQUEST_KEY_ENCRYPTED_TYPE` | 加密类型 |

**关键文件**: `com/baidu/sapi2/result/SmsWapLoginResult.java`

### 2.5 扫码登录流程

```
1. GET /v2/api/getqrcode?client_id=<appid>&...  → 获取二维码图片 + sign
2. 客户端展示二维码
3. 轮询 GET /v2/api/bdusslogin?sign=<sign>&...  → 等待用户扫码确认
4. 返回 BDUSS/STOKEN/pToken
```

#### 扫码参数

| 参数 | 说明 |
|------|------|
| `KEY_QR_LOGIN_CLIENT_ID` | 客户端 app ID |
| `KEY_QR_LOGIN_CMD` | 命令 |
| `KEY_QR_LOGIN_ENCUID` | 加密设备 UID |
| `KEY_QR_LOGIN_LP` | 登录保护标记 |
| `KEY_QR_LOGIN_REDIRECT_URI` | 重定向 URI |
| `KEY_QR_LOGIN_RESPONSE_TYPE` | 响应类型 |
| `KEY_QR_LOGIN_SIGN` | 扫码签名 |

**关键文件**: `com/baidu/sapi2/utils/SapiUtils.java`

### 2.6 一键登录 (手机号)

**响应字段**: `enable`, `encryptPhoneNum`, `hasHistory`, `mobile`, `operator` (CM/CU/CT), `regToken`, `sign`, `secondJsCode`

**关键文件**: `com/baidu/sapi2/result/OneKeyLoginResult.java`

### 2.7 第三方/SSO 登录

支持的社交类型: Weixin, QQ_SSO, Weibo, DingDing, Douyin, OPPO, VIVO, Twitter, YY, CFO, WAUTH

| 参数 | 说明 |
|------|------|
| `type` | 社交类型整数 |
| `appid` | 应用 key |
| `access_token` / `code` | OAuth token 或 auth code |
| `osuid` | 开放平台用户 ID |
| `display` | `"native"` |

**关键文件**: `com/baidu/sapi2/utils/ParamsUtil.java`

### 2.8 密码 RSA 加密

```java
// PwdRSAEncryptor.java
// 算法: RSA, 手动 BigInteger.modPow()
指数: 10001 (hex) = 65537
模数: B3C61EBBA4659C4CE3639287EE871F1F48F7930EA977991C7AFE3CC442FEA496
      43212E7D570C853F368065CC57A2014666DA8AE7D493FD47D171C0D894EEE3ED
      7F99F6798B7FFD7B5873227038AD23E3197631A8CB642213B9F27D4901AB0D92
      BFA27542AE890855396ED92775255C977F5C302F1E7ED4B1E369C12CB6B1822F
```

- 用户名加密: `encryptString(false, username)` — 不做 Base64 预编码
- 密码加密: 先 Base64 编码明文，再 RSA 加密
- 加密后请求附带 `isEncrypted=1`

**关键文件**: `com/baidu/sapi2/PwdRSAEncryptor.java`

### 2.9 账号数据加密 (AES+RSA 混合)

```
加密: base64Encode(aesEncrypt(base64Encode(plaintext), reverse(key), key))
解密: base64Decode(aesDecrypt(base64Decode(ciphertext), reverse(key), key))
RSA: RSA/NONE/PKCS1Padding, 116 字节分块, openapi.baidu.com 证书公钥
```

**关键文件**: `com/baidu/sapi2/utils/SapiDataEncryptor.java`

### 2.10 验证码系统

```
POST https://passport.baidu.com/cap/init
参数: ak=<app_key>&scene=<场景>&ver=2&_=<时间戳>
响应: {"code": 0, "msg": "", "data": {"tk": "...", "as": "...", "ds": "..."}}
```

验证页面: `https://wappass.baidu.com/static/activity/pass-machine.html`
参数: `clientfrom=native&client=android&ak=<key>&type=<type>&scene=<scene>&timestamp=<ts>`

**关键文件**: `com/baidu/validation/ValidationManager.java`

### 2.11 登录响应格式 (XML)

登录 API 返回 XML，通过 `XmlPullParser` 解析:

```xml
<client>
  <error_code>0</error_code>
  <error_description>...</error_description>
  <data>
    <errno>0</errno>
    <uname>username</uname>
    <bduss>SESSION_TOKEN</bduss>
    <ptoken>PERSISTENT_TOKEN</ptoken>
    <stoken>SHORT_TOKEN</stoken>
    <displayname>显示昵称</displayname>
    <uid>用户ID</uid>
    <authsid>auth_session_id</authsid>
    <stoken_list>
      <tpl_name>stoken_value#tpl_name2>stoken_value2</tpl_name>
    </stoken_list>
    <os_headurl>社交头像URL</os_headurl>
    <os_openid>社交OpenID</os_openid>
    <os_name>社交昵称</os_name>
    <os_type>社交类型整数</os_type>
    <incomplete_user>0|1</incomplete_user>
    <actiontype>action</actiontype>
    <fromtype>type</fromtype>
    <livinguname>url_encoded_name</livinguname>
    <loginType>oneKeyLogin</loginType>
    <mobilephone>加密手机号</mobilephone>
  </data>
</client>
```

**响应字段映射** (`SapiAccountResponse`):
`bduss`, `ptoken`, `stoken`, `displayname`, `username`, `email`, `uid`, `portraitSign`, `newReg`, `authSid`, `socialPortraitUrl`, `socialNickname`, `socialType`, `actionType`, `isGuestAccount`, `livingUname`, `app`, `extra`, `accountType`, `fromType`, `tplStokenMap` (Map), `openid`

**成功判断**: `errorCode == 0` 或 (`errorCode == -100` 且 `bduss` 非空)

**关键文件**:
- `com/baidu/sapi2/shell/response/SapiAccountResponse.java`
- `com/baidu/sapi2/utils/SapiCoreUtil.java:197-327` (XML 解析)
- `com/baidu/sapi2/SapiResult.java` (错误码定义, 范围 -801 ~ -200)

### 2.12 H5 通用参数

所有 Web 登录页面附带的通用参数 (`ParamsUtil.buildH5CommonParams()`):

| 参数 | 值 |
|------|-----|
| `clientfrom` | `"native"` |
| `tpl` | Passport 模板名 |
| `client` | `"android"` |
| `adapter` | `"3"` 或 `"8"` |
| `t` | 当前时间戳 (ms) |
| `lang` | `"en"` / `"zh_HK"` |
| `suppcheck` | `"1"` |
| `scanface` | 支持人脸时 `"1"` |
| `liveAbility` | 支持活体时 `"1"` |
| `login_share_strategy` | 分享策略值 |
| `connect` | 统一验证时 `"1"` |
| `disable_voice_vcode` | 禁用语音验证时 `"1"` |

### 2.13 登录 Cookie

登录请求附带的 Cookie (`ParamsUtil.buildNaCookie()`):

| Cookie | 说明 |
|--------|------|
| `cuid` | 设备 ID |
| `DVIF` | 设备信息 |
| `history` | 登录历史 |
| `BAIDUID` | 百度全局用户 ID |
| `ploganondeg` | 匿名登录级别 |

### 2.14 应用配置

| 常量 | 值 | 来源 |
|------|-----|------|
| `SDK_APP_KEY` | `"200019"` | `AccountUtils.java:63` |
| `SDK_WHO` | `200019` | `AccountUtils.java` |
| `VERSION_NAME` | `"9.15.1.32"` | `SapiAccountManager.java` |
| `VERSION_CODE` | `250` | `SapiAccountManager.java` |

---

## 3. API 域名体系

### 3.1 网盘 API 域名

| 域名 | 用途 | 代码来源 |
|------|------|----------|
| `https://pan.baidu.com/api/` | 主 API (文件操作、配额等) | `CommonServerURL.DEFAULT_HTTPS_HOST_NAME` |
| `https://pan.baidu.com/rest/2.0/pcs/` | PCS 文件存储 API | `TransmitterBaseURL.DEFAULT_PCS_HOST_NAME` |
| `https://pan.baidu.com/rest/2.0/xpan/` | XPan 扩展 API | `CloudFileServerURL.DEFAULT_XPAN_HOST_NAME` |
| `https://d.pcs.baidu.com/rest/2.0/pcs/file` | 下载/上传 PCS 节点 | `TransmitterBaseURL.getDPCSHostName()` |
| `https://c.pcs.baidu.com` | 上传 CDN 节点 | `CommonServerURL.CDN_UPLOAD_PCS_DOMAIN` |
| `https://pan.baidu.com/rest/` | REST API | `CommonServerURL.DEFAULT_REST_HOST_NAME` |
| `https://pan.baidu.com/rest/2.0/dss/` | DSS API | `CommonServerURL.DEFAULT_DSS_HOST_NAME` |

### 3.2 登录/Passport 域名

| 域名 | 用途 | 代码来源 |
|------|------|----------|
| `https://passport.baidu.com` | 主登录 API | `SapiHost.DOMAIN_ONLINE_PASSPORT_URL` |
| `https://wappass.baidu.com` | WAP 登录 | `SapiHost.DOMAIN_ONLINE_WAPPASS_URL` |
| `https://wappass.bdimg.com` | 配置 CDN | `SapiHost.DOMAIN_ONLINE_CONFIG_HTTPS_URL` |
| `https://openapi.baidu.com` | OAuth 设备 API | `SapiHost.DOMAIN_ONLINE_DEVICE_URL` |

**关键常量**:
- `PCS_APP_ID = "250528"` (网盘)
- `PCS_APP_ID_YOUTH = "25179614"` (青少年版)
- `app_id` 参数用于标识客户端身份

---

## 4. 下载接口 (Download)

### 4.1 定位下载服务器

```
GET https://d.pcs.baidu.com/rest/2.0/pcs/file?method=locatedownload
    &path=<url_encoded_path>
    &ver=2.0
    &dtype=0
    &esl=1
    &ehps=<0|1>
    &app_id=250528
    &check_blue=1
```

**可选参数**:
| 参数 | 说明 |
|------|------|
| `token` | 加速 token |
| `timestamp` | 时间戳 |
| `revision` | 版本修订 |
| `product_type` | 产品类型 (如 `workspace`) |
| `dpkg` | 数据包标识 (-1 或 1) |
| `sd` | 可用流量 |

**代码来源**: `TransferApi.getLocateDownload()` / `TransferApi.getDlinkLocateDownload()`
**文件**: `com/baidu/netdisk/transfer/io/TransferApi.java:193-242`

### 4.2 下载 URL 模板

```
%s%s/rest/2.0/pcs/file?method=%s&path=%s&app_id=250528&ec=1&check_blue=1
```

**代码来源**: `TransmitterBaseURL.DOWNLOAD_URL`

### 4.3 下载流程

```
1. 调用 locatedownload 获取下载服务器列表
2. 从响应中选取最优服务器 URL
3. 携带 BDUSS Cookie 直接 GET 下载
```

### 4.4 视频流播放

```
GET https://pan.baidu.com/api/streaming?no_report_preview=1&
    check_blue=1&
    <fsid|path>=<value>&
    type=<stream_type>&
    ehps=<0|1>&
    app_id=250528
```

**代码来源**: `TransmitterBaseURL.VIDEO_PLAY_API`

---

## 5. 上传接口 (Upload)

### 5.1 上传流程 (多步骤)

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  PreCreate   │────▶│ LocateUpload │────▶│  SuperFile2  │────▶│   Create     │
│  (预创建)     │     │  (定位上传)   │     │  (分片上传)   │     │  (创建文件)   │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
```

### 5.2 PreCreate (预创建)

```
POST https://pan.baidu.com/api/precreate
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `path` | string | 目标路径 |
| `size` | long | 文件大小 |
| `isdir` | int | 是否目录 (0) |
| `block_list` | JSON | 分片 MD5 列表 |
| `rtype` | int | 冲突策略 (0=重命名, 1=覆盖, 2=跳过) |
| `content-md5` | string | 文件总 MD5 (可选) |

**响应**: 返回 `uploadid`, `block_list` (需上传的分片索引)

**代码来源**: `TransferUploadApi.preCreateFile()`
**文件**: `com/baidu/netdisk/transfer/TransferUploadApi.java`

### 5.3 LocateUpload (定位上传服务器)

```
GET https://d.pcs.baidu.com/rest/2.0/pcs/file?method=locateupload
    &uploadsign=<sign>
    &upload_version=2.0
    &app_id=250528
```

**响应**: 返回上传服务器地址

**代码来源**: `TransferUploadApi.getLocateUpload()`

### 5.4 SuperFile2 Upload (分片上传)

```
POST https://d.pcs.baidu.com/rest/2.0/pcs/superfile2?method=upload
    &type=tmpfile
    &path=<path>
    &partoffset=<offset>
    &app_id=250528
    &uploadid=<uploadid>
    &partseq=<part_seq>
```

**请求体**: 分片二进制数据 (multipart/form-data)

**代码来源**: `TransmitterBaseURL.UPLOAD_TMPFILE_URL`

### 5.5 Create (创建文件 / 完成上传)

```
POST https://pan.baidu.com/api/create
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `path` | string | 目标路径 |
| `size` | long | 文件大小 |
| `isdir` | int | 0 |
| `block_list` | JSON | 分片 MD5 列表 |
| `uploadid` | string | 预创建返回的 uploadid |
| `rtype` | int | 冲突策略 |

**代码来源**: `TransferUploadApi.createFile()`

### 5.6 RapidUpload (秒传)

```
POST https://pan.baidu.com/api/rapidUpload
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `path` | string | 目标路径 |
| `content-length` | long | 文件大小 |
| `content-md5` | string | 文件 MD5 |
| `slice-md5` | string | 前 256KB MD5 |
| `rtype` | int | 冲突策略 |

**代码来源**: `TransferUploadApi.rapidUpload()`

### 5.7 查询分片列表

```
POST https://pan.baidu.com/api/batch/getpartlist
```

**代码来源**: `TransferUploadApi.partList()`

### 5.8 工作区上传 (多人协作)

```
POST https://pan.baidu.com/api/ostp/file/upload/precreate?sharedSpaceId=<id>
POST https://pan.baidu.com/api/ostp/file/upload/create?sharedSpaceId=<id>&rtype=<rtype>
POST https://pan.baidu.com/api/ostp/file/upload/rapidupload?sharedSpaceId=<id>
```

---

## 6. 文件管理接口 (File Operations)

### 6.1 统一文件管理入口

所有文件操作通过 `filemanager` 端点的 `opera` 参数区分:

```
POST https://pan.baidu.com/api/filemanager?opera=<operation>
```

**认证**: BDUSS Cookie + `bdstoken` 查询参数 (无需额外 sign)

### 6.2 重命名 (Rename)

```
POST https://pan.baidu.com/api/filemanager?opera=rename&async=2&ondup=1
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `filelist` | JSON | `[{"path":"/原路径/文件名","newname":"新名字"}]` |
| `async` | int | 异步模式 (2) |
| `ondup` | int | 冲突策略 |

**代码来源**: `CloudFileApi.sendRenameRequest()`
**文件**: `com/baidu/netdisk/cloudfile/io/CloudFileApi.java:905-929`
**解析器**: `com/baidu/netdisk/cloudfile/io/parser/FileRenameParser.java`

### 6.3 移动 (Move)

```
POST https://pan.baidu.com/api/filemanager?opera=move&async=2&onnest=1&ondup=1
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `filelist` | JSON | `[{"path":"/原路径","dest":"/目标目录","newname":"新名字","ondup":"..."}]` |
| `async` | int | 异步模式 |
| `onnest` | int | 嵌套策略 |
| `ondup` | int | 冲突策略 |

**代码来源**: `CloudFileApi.sendMoveRequest()`
**文件**: `com/baidu/netdisk/cloudfile/io/CloudFileApi.java:1161-1201`
**解析器**: `com/baidu/netdisk/cloudfile/io/parser/FileMoveParser.java`

### 6.4 复制 (Copy)

```
POST https://pan.baidu.com/api/filemanager?opera=copy&async=2&ondup=1
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `filelist` | JSON | `[{"path":"/原路径","dest":"/目标目录","newname":"新名字"}]` |

**代码来源**: `CloudFileApi.sendCopyRequest()`
**文件**: `com/baidu/netdisk/cloudfile/io/CloudFileApi.java:1122-1159`

### 6.5 删除 (Delete)

```
POST https://pan.baidu.com/api/filemanager?opera=delete
```

> URL 常量: `IMLogKt.CHECK_API3 = "filemanager?opera=delete"`
> (`com/baidu/netdisk/cloudp2p/IMLogKt.java:20`)

#### 请求参数

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `filelist` | JSON | 是 | `["/路径1", "/路径2"]` (路径字符串数组) |
| `async` | int | 否 | 异步模式: 0=同步, 1=异步, 2=异步(推荐) |
| `ondup` | string | 否 | 冲突策略 |
| `captchaidentity` | string | 否 | 验证码身份标识 (二次验证时传入) |
| `captchatype` | string | 否 | 验证码类型 (二次验证时传入) |
| `product_type` | string | 否 | `"workspace"` (仅工作区文件) |

#### 方法签名

```java
// CloudFileApi.java:395
public FileManagerResult delete(
    List<String> list,    // 待删除文件路径列表
    int i,                // async 模式 (0/1/2)
    String str,           // ondup 冲突策略
    String str2,          // captchaidentity
    String str3           // captchatype
)
```

**代码来源**: `CloudFileApi.delete()` → `sendDeleteRequest()`
**文件**: `com/baidu/netdisk/cloudfile/io/CloudFileApi.java:395-421`
**解析器**: `com/baidu/netdisk/cloudfile/io/parser/DeleteParser.java`

#### 响应解析逻辑 (DeleteParser)

1. JSON 反序列化为 `ManageResponse`
2. 若 `errno != 0` 或 `taskid <= 0`，遍历 `info[]` 数组:
   - `errno` 为 `-9`、`31066` 或 `0` → 该文件视为删除成功，加入成功路径列表
   - 其他 `errno` → 记录错误
3. 若 `errno == 132` 且 `authwidget` 存在 (`saferand`/`safetpl`/`safesign` 非空) → 抛出 `NeedAuthWidgetException`，需要安全验证二次确认
4. 若 `errno == -19` → 需要验证码 (`NEED_VERIFY_CODE`)，构造 `VerifyParam` (含 `type`, `key`, `msg`)
5. 构造 `FileManagerResult` 返回

#### 删除响应示例

```json
{
  "errno": 0,
  "errmsg": "success",
  "taskid": 123456,
  "info": [
    {"errno": 0, "path": "/folder/file.txt"},
    {"errno": 0, "path": "/folder/file2.txt"}
  ],
  "target_file_nums": 2
}
```

**errno == 132 时 (需要安全验证)**:

```json
{
  "errno": 132,
  "authwidget": {
    "saferand": "随机串",
    "safesign": "签名",
    "safetpl": "验证模板"
  },
  "verify_scene": "验证场景"
}
```

### 6.6 文件管理通用响应格式 (ManageResponse)

重命名/移动/复制/删除操作共用此响应结构:

```json
{
  "errno": 0,
  "errmsg": "success",
  "taskid": 123456,
  "task_id": 789,
  "info": [
    {"errno": 0, "path": "/folder/file.txt", "newno": "..."}
  ],
  "target_file_nums": 1,
  "authwidget": {
    "saferand": "...",
    "safesign": "...",
    "safetpl": "..."
  },
  "ckey": "...",
  "ctype": "...",
  "cmsg": "验证提示文本",
  "duplicated": {"list": [...], "total": 0},
  "verify_scene": "...",
  "num_limit": 0,
  "path": "/result/path"
}
```

**响应字段映射** (`ManageResponse`):

| JSON 字段 | Java 字段 | 类型 | 说明 |
|-----------|-----------|------|------|
| `errno` | `errno` | int | 0=成功 |
| `errmsg` | `errmsg` | String | 错误消息 |
| `show_msg` | `alertmsg` | String | 前端展示消息 |
| `taskid` | `taskid` | long | 异步任务 ID |
| `task_id` | `saveTaskid` | long | 保存任务 ID |
| `info` | `f52675info` | InfoResponse[] | 各文件操作结果 |
| `target_file_nums` | `fileNumber` | int | 目标文件数 |
| `authwidget` | `mAuthWidget` | AuthWidget | 二次验证组件 |
| `ckey` | `mCKey` | String | 验证 key |
| `ctype` | `mCType` | String | 验证类型 |
| `cmsg` | `mVerifyDialogTips` | String | 验证对话框提示 |
| `duplicated` | `mduplicated` | object | 重复文件信息 |
| `verify_scene` | `verifyScene` | String | 验证场景 |
| `num_limit` | `numLimit` | int | 数量限制 |

**InfoResponse 子项**:

| JSON 字段 | Java 字段 | 类型 | 说明 |
|-----------|-----------|------|------|
| `errno` | `errno` | int | 单项结果码 |
| `path` | `path` | String | 文件路径 |
| `newno` | `newno` | String | 新编号 |

**解析逻辑**:
- 重命名: 顶层 `errno == 0` 且 `taskid > 0` 则成功；否则遍历 `info[]` 检查各项 `errno`
- 移动/复制: 包装为 `FileManagerResult`，收集失败的 `InfoResponse` 到错误列表
- 删除: `errno == 132` 触发二次验证；`errno` 为 -9/31066/0 视为删除成功

**关键文件**:
- `com/baidu/netdisk/cloudfile/io/model/ManageResponse.java`
- `com/baidu/netdisk/cloudfile/io/model/InfoResponse.java`
- `com/baidu/netdisk/cloudfile/io/model/FileManagerResult.java`

### 6.7 XPan 文件管理

```
POST https://pan.baidu.com/rest/2.0/xpan/multimedia?method=filemanager
```

**代码来源**: `CloudFileApi`

---

## 7. 空间配额 (Quota)

```
GET https://pan.baidu.com/api/quota
    &checkrecycle=<0|1>
    &needquota=<0|1>
    &extra_appid=<appid>
```

**响应字段**:
| 字段 | 说明 |
|------|------|
| `total` | 总空间 (字节) |
| `used` | 已用空间 (字节) |
| `free` | 剩余空间 (字节) |
| `expire` | 过期时间 |

**代码来源**: `CloudFileApi.getQuota()`
**文件**: `com/baidu/netdisk/cloudfile/io/CloudFileApi.java`

---

## 8. 文件列表 (List Directory)

### 8.1 请求

```
GET https://pan.baidu.com/api/list
    &dir=<path>
    &start=<offset>
    &limit=<count>
    &order=<name|time|size>
    &desc=<0|1>
    &preset=<0|1>
    &isdir=<0|1>
```

**代码来源**: `CloudFileApi.getDirectoryAttribute()` / `CloudFileApi.getList()`
**文件**: `com/baidu/netdisk/cloudfile/io/CloudFileApi.java:543-585`
**解析器**: `com/baidu/netdisk/cloudfile/io/parser/GetDirectoryListParser.java`

### 8.2 响应格式

```json
{
  "errno": 0,
  "errmsg": "",
  "list": [
    {
      "fs_id": 123456789,
      "server_filename": "example.txt",
      "isdir": 0,
      "path": "/example.txt",
      "size": 1024,
      "md5": "abc123...",
      "server_ctime": 1700000000,
      "server_mtime": 1700000000,
      "server_atime": 1700000000,
      "local_ctime": 1700000000,
      "local_mtime": 1700000000,
      "category": 6,
      "share": 0,
      "dlink": "https://d.pcs.baidu.com/...",
      "is_collect": 0,
      "updated_status": 0,
      "height": 0,
      "width": 0,
      "duration": 0,
      "uk": 0,
      "oper_id": 0,
      "resolution": "",
      "cover": "",
      "wpfile": 0,
      "pl": 0,
      "permit_code": 0,
      "permit_name": "",
      "real_category": 6,
      "owner_id": 0,
      "thumbs": {"url1": "https://..."}
    }
  ]
}
```

### 8.3 文件字段完整映射 (CloudFile)

| JSON 字段 | Java 字段 | 类型 | 说明 |
|-----------|-----------|------|------|
| `fs_id` | `id` | long | 文件唯一 ID |
| `server_filename` | `filename` | String | 文件名 |
| `isdir` | `isDir` | int | 0=文件, 1=目录 |
| `path` | `path` | String | 完整路径 |
| `size` | `size` | long | 文件大小 (字节) |
| `md5` | `md5` | String | 文件 MD5 |
| `server_ctime` | `serverCTime` | long | 服务端创建时间 (unix) |
| `server_mtime` | `serverMTime` | long | 服务端修改时间 (unix) |
| `server_atime` | `serverATime` | long | 服务端访问时间 (unix) |
| `local_ctime` | `localCTime` | long | 本地创建时间 (unix) |
| `local_mtime` | `localMTime` | long | 本地修改时间 (unix) |
| `category` | `category` | int | 文件分类 |
| `share` | `property` | int | 分享属性 |
| `dlink` | `dlink` | String | 下载链接 |
| `is_collect` | `fileIsCollection` | int | 是否收藏 |
| `updated_status` | `fileTag` | int | 更新状态 |
| `extent_int8` | `extentLong` | long | 扩展字段 |
| `height` | `mImageHeight` | int | 图片高度 |
| `width` | `mImageWidth` | int | 图片宽度 |
| `duration` | `mDuration` | long | 视频/音频时长 |
| `uk` | `mOwnerUK` | long | 上传者 UK |
| `oper_id` | `mOperatorUK` | long | 操作者 UK |
| `gid` | `mGid` | long | 组 ID |
| `resolution` | `resolution` | String | 视频分辨率 |
| `cover` | `cover` | String | 封面 URL |
| `wpfile` | `wpfile` | int | WP 文件标记 |
| `pl` | `pl` | int | PL 标记 |
| `permit_code` | `permitCode` | int | 权限码 |
| `permit_name` | `permitName` | String | 权限名 |
| `permit_source` | `permitSource` | String | 权限来源 |
| `origin_video_md5` | `originVideoMd5` | String | 原始视频 MD5 |
| `video_compress_quality` | `videoCompressQuality` | String | 视频压缩质量 |
| `picdocpreview` | `picdocpreview` | int | 图片文档预览 |
| `scene` | `scene` | int | 场景 |
| `share_id` | `shareId` | long | 分享 ID |
| `share_uk` | `shareUk` | String | 分享 UK |
| `context` | `content` | String | 搜索上下文 |
| `delete_source` | `deleteSource` | int | 删除来源 |
| `action_info` | `actionInfo` | String | 操作信息 |
| `real_category` | `realCategory` | int | 真实分类 |
| `owner_id` | `ownerId` | long | 所有者 ID |
| `tkbind_id` | `tkbindId` | long | 绑定 ID |
| `instance_json` | `instanceJson` | String | 实例 JSON |
| `thumbs` | `thumbs` | object | 缩略图 (含 `url1`) |

**基础响应字段** (`Response`):

| JSON 字段 | Java 字段 | 类型 | 说明 |
|-----------|-----------|------|------|
| `errno` | `errno` | int | 0=成功 |
| `errmsg` | `errmsg` | String | 错误消息 |
| `show_msg` | `alertmsg` | String | 前端展示消息 |
| `prompt_type` | `promptType` | int | 提示类型 |

**关键文件**:
- `com/baidu/netdisk/cloudfile/io/model/CloudFile.java`
- `com/baidu/netdisk/cloudfile/io/model/GetDirectoryListResponse.java`
- `com/baidu/netdisk/network/response/Response.java`

---

## 9. 搜索 (Search)

```
GET https://pan.baidu.com/api/search
    &key=<keyword>
    &dir=<path>
    &category=<category>
    &recursion=<0|1>
```

**代码来源**: `CloudFileApi.search()`

---

## 10. 创建目录 (Create Directory)

```
POST https://pan.baidu.com/api/create?a=commit[&norename]
```

> `&norename` 为可选参数，传入后服务端不会对同名目录自动重命名

### 10.1 请求参数

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `path` | string | 是 | 目录路径 (如 `/我的资源/新建文件夹`) |
| `size` | int | 是 | 固定 `0` |
| `isdir` | int | 是 | 固定 `1` |
| `local_ctime` | timestamp | 是 | 本地创建时间 (unix 时间戳) |
| `local_mtime` | timestamp | 是 | 本地修改时间 (unix 时间戳) |
| `block_list` | JSON | 是 | 固定 `[]` |
| `product_type` | string | 否 | `"workspace"` (工作区文件) |
| `cid` | string | 否 | 企业版 CID (仅 `networkSpaceType == ENTERPRISE` 时) |

### 10.2 方法签名

```java
// CloudFileApi.java:357
public CreateFileResponse createDirectory(
    String str,              // 目录路径
    long j,                  // 时间戳 (同时用于 local_ctime 和 local_mtime)
    List<String> list,       // block list (通常为空列表)
    boolean z,               // norename 标记
    NetworkSpaceType type,   // null=普通, ENTERPRISE=企业版
    String str2              // 企业版 cid
)
```

### 10.3 响应格式 (CreateFileResponse)

```json
{
  "errno": 0,
  "errmsg": "",
  "fs_id": 123456789,
  "path": "/我的资源/新建文件夹",
  "isdir": 1,
  "mtime": 1700000000,
  "ctime": 1700000000,
  "status": 0
}
```

**响应字段映射**:

| JSON 字段 | Java 字段 | 类型 | 说明 |
|-----------|-----------|------|------|
| `errno` | `errno` | int | 0=成功 |
| `errmsg` | `errmsg` | String | 错误消息 |
| `show_msg` | `alertmsg` | String | 前端展示消息 |
| `prompt_type` | `promptType` | int | 提示类型 |
| `fs_id` | `fid` | long | 新建目录的文件系统 ID |
| `path` | `path` | String | 目录路径 |
| `isdir` | `isdir` | int | 1 (目录) |
| `mtime` | `mtime` | long | 修改时间 (unix) |
| `ctime` | `ctime` | long | 创建时间 (unix) |
| `status` | `status` | int | 状态码 |

### 10.4 响应解析逻辑 (CreateDirectoryParser)

1. JSON 反序列化为 `CreateFileResponse`
2. 若解析结果为 null → 抛出 `JSONException`
3. 若 `errno == 0` → 返回 `CreateFileResponse` (含 `fs_id`, `path` 等)
4. 若 `errno != 0` → 抛出 `RemoteException`

**代码来源**: `CloudFileApi.sendCreateDirRequest()` (lines 190-216)
**解析器**: `com/baidu/netdisk/cloudfile/io/parser/CreateDirectoryParser.java`
**响应模型**: `com/baidu/netdisk/cloudfile/io/model/CreateFileResponse.java`

---

## 11. 其他接口

### 11.1 文件差异 (Diff)

```
GET https://pan.baidu.com/api/filediff
```

### 11.2 目录大小

```
GET https://pan.baidu.com/api/dirsize
```

### 11.3 分类列表

```
GET https://pan.baidu.com/api/categorylist
```

### 11.4 文件收藏

```
GET https://pan.baidu.com/api/addfilecollection?fs_id=<fs_id>
GET https://pan.baidu.com/api/cancelfilecollection?fsid_list=<ids>
```

### 11.5 隐藏文件

```
POST https://pan.baidu.com/rest/2.0/xpan/multimedia
    method=sethidden
    &flag=<0|1>
    &fs_ids=<id_list>
```

### 11.6 文件申诉

```
POST https://pan.baidu.com/api/fileappeal/add
    &appeal_channel=<channel>
    &files_info=<json>
```

---

## 12. 关键源码文件索引

### 网络/认证层

| 文件 | 说明 |
|------|------|
| `com/baidu/netdisk/network/BaseApi.java` | API 基类 (bdstoken 计算, 请求参数注入) |
| `com/baidu/netdisk/base/network/CommonServerURL.java` | 核心 URL 配置 |
| `com/baidu/netdisk/base/network/CloudFileServerURL.java` | 云文件 URL 配置 |
| `com/baidu/netdisk/transfer/transmitter/base/TransmitterBaseURL.java` | 传输 URL 配置 |
| `com/baidu/netdisk/base/network/NetworkTaskWrapper.java` | 网络任务执行器 |
| `com/baidu/netdisk/kernel/architecture/AppCommon.java` | 全局常量 (PCS_APP_ID 等) |

### 登录/账号

| 文件 | 说明 |
|------|------|
| `com/baidu/sapi2/utils/SapiEnv.java` | 所有登录端点 URI 常量 |
| `com/baidu/sapi2/utils/SapiHost.java` | 登录域名配置 (Base64 编码) |
| `com/baidu/sapi2/utils/SapiUtils.java` | 扫码登录参数 |
| `com/baidu/sapi2/PwdRSAEncryptor.java` | 密码 RSA 加密 (公钥/指数/模数) |
| `com/baidu/sapi2/utils/SapiDataEncryptor.java` | AES+RSA 混合加密 |
| `com/baidu/sapi2/utils/ParamsUtil.java` | H5 通用参数 / NA Cookie 构造 |
| `com/baidu/sapi2/shell/response/SapiAccountResponse.java` | 登录响应字段映射 |
| `com/baidu/sapi2/utils/SapiCoreUtil.java` | 登录 XML 响应解析 |
| `com/baidu/sapi2/SapiResult.java` | 错误码定义 (-801 ~ -200) |
| `com/baidu/sapi2/result/AccountAndPwdLoginResult.java` | 账密登录参数常量 |
| `com/baidu/sapi2/result/SmsWapLoginResult.java` | 短信登录参数 |
| `com/baidu/sapi2/result/OneKeyLoginResult.java` | 一键登录响应 |
| `com/baidu/validation/ValidationManager.java` | 验证码系统 |
| `com/baidu/netdisk/account/AccountUtils.java` | 账号管理 (BDUSS/STOKEN) |
| `com/baidu/netdisk/util/CookieUtils.java` | Cookie 构造 |
| `com/baidu/netdisk/account/AccountServerURL.java` | Passport URL 配置 |

### 文件操作

| 文件 | 说明 |
|------|------|
| `com/baidu/netdisk/cloudfile/io/CloudFileApi.java` | 核心文件操作 API (1,217 行) |
| `com/baidu/netdisk/cloudfile/io/parser/GetDirectoryListParser.java` | 文件列表解析 |
| `com/baidu/netdisk/cloudfile/io/parser/FileRenameParser.java` | 重命名响应解析 |
| `com/baidu/netdisk/cloudfile/io/parser/FileMoveParser.java` | 移动响应解析 |
| `com/baidu/netdisk/cloudfile/io/parser/DeleteParser.java` | 删除响应解析 |
| `com/baidu/netdisk/cloudfile/io/parser/CreateDirectoryParser.java` | 创建目录响应解析 |
| `com/baidu/netdisk/cloudfile/io/model/CloudFile.java` | 文件数据模型 (30+ 字段) |
| `com/baidu/netdisk/cloudfile/io/model/ManageResponse.java` | 文件操作通用响应 |
| `com/baidu/netdisk/cloudfile/io/model/InfoResponse.java` | 单项操作结果 |
| `com/baidu/netdisk/cloudfile/io/model/FileManagerResult.java` | 文件管理结果包装 |
| `com/baidu/netdisk/cloudfile/io/model/CreateFileResponse.java` | 创建文件/目录响应 (fs_id, path, ctime, mtime) |
| `com/baidu/netdisk/cloudfile/io/model/GetDirectoryListResponse.java` | 列表响应包装 |
| `com/baidu/netdisk/network/response/Response.java` | 基础响应 (errno/errmsg) |

### 传输 (上传/下载)

| 文件 | 说明 |
|------|------|
| `com/baidu/netdisk/transfer/TransferUploadApi.java` | 上传 API |
| `com/baidu/netdisk/transfer/io/TransferApi.java` | 下载 API |

### 加密/签名

| 文件 | 说明 |
|------|------|
| `com/baidu/netdisk/utils/encode/SHA1Util.java` | HMAC-SHA1 实现 + 密钥派生 |
| `com/baidu/netdisk/utils/encode/MD5Util.java` | MD5 工具 |

---

## 13. Frida Hook 建议

### 13.1 获取 BDUSS

```javascript
Java.perform(function() {
    var AccountUtils = Java.use("com.baidu.netdisk.account.AccountUtils");
    var instance = AccountUtils.getInstance();
    console.log("BDUSS:", instance.getBduss());
    console.log("STOKEN:", instance.getStoken());
    console.log("UID:", instance.getUid());
});
```

### 13.2 Hook 下载请求

```javascript
Java.perform(function() {
    var TransferApi = Java.use("com.baidu.netdisk.transfer.io.TransferApi");
    TransferApi.getLocateDownload.implementation = function(path, preview, token, ts, rev, pt, z2) {
        console.log("[Download] path:", path);
        var result = this.getLocateDownload(path, preview, token, ts, rev, pt, z2);
        console.log("[Download] result:", JSON.stringify(result));
        return result;
    };
});
```

### 13.3 Hook 上传请求

```javascript
Java.perform(function() {
    var UploadApi = Java.use("com.baidu.netdisk.transfer.TransferUploadApi");
    
    UploadApi.preCreateFile.implementation = function(params) {
        console.log("[Upload PreCreate]", params.toString());
        return this.preCreateFile(params);
    };
    
    UploadApi.createFile.implementation = function(params, str) {
        console.log("[Upload Create]", params.toString(), str);
        return this.createFile(params, str);
    };
});
```

### 13.4 Hook 文件操作

```javascript
Java.perform(function() {
    var CloudFileApi = Java.use("com.baidu.netdisk.cloudfile.io.CloudFileApi");
    
    // Hook rename
    CloudFileApi.sendRenameRequest.overload('java.lang.String', 'java.lang.String', 'java.lang.String', 'boolean').implementation = function() {
        console.log("[Rename]", arguments);
        return this.sendRenameRequest.apply(this, arguments);
    };
});
```

### 13.5 Hook 网络层 (通用)

```javascript
Java.perform(function() {
    var NetworkTaskWrapper = Java.use("com.baidu.netdisk.base.network.NetworkTaskWrapper");
    NetworkTaskWrapper.send.overload('[Lcom.baidu.netdisk.network.request.HttpRequest;').implementation = function(requests) {
        for (var i = 0; i < requests.length; i++) {
            console.log("[HTTP]", requests[i].getMethod(), requests[i].getUrl());
        }
        return this.send.apply(this, arguments);
    };
});
```

### 13.6 Hook 登录响应

```javascript
Java.perform(function() {
    var SapiCoreUtil = Java.use("com.baidu.sapi2.utils.SapiCoreUtil");
    // Hook XML 解析以捕获登录结果
    // SapiCoreUtil 的响应解析方法 (lines 197-327)
});
```

### 13.7 Hook 密码加密

```javascript
Java.perform(function() {
    var PwdRSAEncryptor = Java.use("com.baidu.sapi2.PwdRSAEncryptor");
    PwdRSAEncryptor.encryptString.implementation = function(preEncode, plaintext) {
        console.log("[RSA Encrypt] preEncode:", preEncode, "plain:", plaintext);
        var result = this.encryptString(preEncode, plaintext);
        console.log("[RSA Encrypt] result:", result);
        return result;
    };
});
```
