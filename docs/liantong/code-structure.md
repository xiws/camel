# 联通云盘 (Liantong Cloud Disk) - 代码结构分析

## 基本信息

- **包名**: `com.chinaunicom.bol.cloudapp`
- **版本**: 6.2.0 (versionCode: 62001)
- **加固**: 梆梆加固 (Secneo)
- **目标 SDK**: 35 (Android 15)
- **最低 SDK**: 23 (Android 6.0)

## 保护机制

该 APK 使用梆梆加固 (Secneo) 进行保护：
- `libDexHelper.so` / `libDexHelper-x86.so` - DEX 解密辅助库
- `libdexjni.so` - DEX JNI 接口
- `libEncryptJni.so` / `libEncryptJniTf.so` - 加密 JNI 库
- `libNetHTProtect.so` - 网络保护库
- `libzxprotect.so` - 核心保护库

实际业务代码在运行时动态解密，静态分析只能看到壳代码。

## 模块结构 (基于 ARouter 路由)

### 主要模块

```
com.chinaunicom.bol.cloudapp/
├── v.activity/                    # UI 层
│   ├── WelcomeActivity           # 启动页
│   └── MainActivity              # 主界面
│
├── module.account/               # 账户模块
│   └── v.activity/
│       ├── LoginActivity         # 登录
│       └── SetPwdActivity        # 设置密码
│
├── module.cloud/                 # 云盘核心模块
│   ├── ui/
│   │   ├── cloudfile/           # 云文件管理
│   │   │   └── CloudFileFragment
│   │   ├── filelist/            # 文件列表
│   │   │   └── BaseFileListActivity
│   │   ├── choosefile/          # 文件选择
│   │   │   ├── ChoiceFileActivity
│   │   │   ├── ChoiceAudioActivity
│   │   │   └── CloudCameraActivity
│   │   ├── transfer/            # 传输列表
│   │   │   ├── TransferListActivity
│   │   │   └── QuantumTransferListActivity
│   │   ├── preview/             # 文件预览
│   │   │   ├── PreviewImageActivity
│   │   │   └── PreviewAIImageActivity
│   │   ├── search/              # 搜索
│   │   │   ├── SearchFileActivity
│   │   │   └── SearchGalleryResultActivity
│   │   ├── recyclebin/          # 回收站
│   │   │   └── RecycleBinActivity
│   │   ├── sharemanage/         # 分享管理
│   │   │   └── ShareManageActivity
│   │   ├── secretspace/         # 保密空间
│   │   │   └── SecretSpaceActivity
│   │   ├── document/            # 文档管理
│   │   │   └── DocumentActivity
│   │   ├── personalcloud/       # 个人云盘
│   │   │   ├── PersonalCloudFragment
│   │   │   └── CommonFileListActivity
│   │   ├── familycloud/         # 家庭云
│   │   │   └── FamilyCloudFragmentV1
│   │   ├── campuscloud/         # 校园云
│   │   │   ├── CampusListActivity
│   │   │   └── CampusDetailActivity
│   │   ├── gallerybackup/       # 相册备份
│   │   │   ├── GalleryBackupListActivity
│   │   │   └── GalleryBackupSettingActivity
│   │   ├── intelligentdelete/   # 智能清理
│   │   │   └── IntelligentCleanActivity
│   │   └── ...
│   └── aroute/
│       └── CloudArouteServiceImpl
│
├── module_im/                    # 即时通讯模块
│   ├── contactkit/
│   │   └── ui/ContactUIService
│   ├── chatkit/
│   │   └── ui/ChatUIService
│   └── conversation/
│       └── ui/ConversationUIService
│
└── module.home/                  # 首页模块
```

### ARouter 路由表

#### 账户模块 (/account)
- `/account/login` → LoginActivity
- `/account/SetPwdActivity` → SetPwdActivity
- `/account/accountService` → 账户服务

#### 云盘模块 (/cloud)
- `/cloud/main` → MainActivity
- `/cloud/CloudFileFragment` → 云文件片段
- `/cloud/TransferListActivity` → 传输列表
- `/cloud/BaseFilelListActivity` → 基础文件列表
- `/cloud/ChooseCloudFileActivity` → 选择云文件
- `/cloud/RecycleBinActivity` → 回收站
- `/cloud/SearchFileActivity` → 搜索文件
- `/cloud/SecretSpaceActivity` → 保密空间
- `/cloud/ShareManageActivity` → 分享管理
- `/cloud/cloudService` → 云服务

## 核心类 (从字符串分析)

### 文件操作相关
- `com.cloud.wolibrary.upload.UploadTask` - 上传任务
- `com.cloud.wolibrary.manager.UploadManager` - 上传管理器
- `com.cloud.wolibrary.request.UploadRequest` - 上传请求
- `com.cloud.wolibrary.bean.UploadProgress` - 上传进度
- `com.cloud.wolibrary.UploadSwitchManager` - 上传开关管理
- `com.lzy.okserver.download.DownloadTask` - 下载任务
- `com.lzy.okgo.db.DownloadManager` - 下载管理器

### 文件列表相关
- `AllFileListRepository.kt` - 所有文件仓库
- `BaseFileListRepository.kt` - 基础文件列表仓库
- `AudioBackupFileListRepository.kt` - 音频备份文件列表
- `CollectFileListRepository.kt` - 收藏文件列表
- `MultiMediaFileListRepository.kt` - 多媒体文件列表
- `FileListViewModel.kt` - 文件列表 ViewModel
- `FileListSelectionController.kt` - 文件列表选择控制器

### 请求相关
- `QueryFileListCountRequest.kt` - 查询文件列表数量
- `UploadTempFileRequest.kt` - 上传临时文件
- `UploadOcrRequest.kt` - OCR 上传请求
- `FilterTypeFilesRequest.kt` - 过滤类型文件
- `CloudGalleryFilterOptionRequest.kt` - 云相册过滤选项

### 请求封装（dispatcher）

业务请求经过统一的 dispatcher 封装层（包名 `com.chinaunicom.bol.cloud.repository.bean.request`），
典型请求 bean：`DeleteFileRequest`、`RenameFileOrDirectoryRequest`、`CreateDirectoryRequest`、
`MoveFile`/`CopyFile`、`GetDownloadUrlRequest(V2)`、`FetchFileDownloadUrlReq`、
`FileBatchManageRequest`、`AppLoginByMobileRequest`、`AppLoginRequest`。

这些类的方法体在梆梆加固中被清空，无法直接读出字段；实际字段是通过网页版 bundle
逆向 + 真机实测确认的，见 [dispatcher-protocol.md](dispatcher-protocol.md)。

## 第三方库

- **ARouter** - 阿里路由框架
- **OkGo/OkServer** - 网络请求和文件下载
- **MMKV** - 腾讯高性能 KV 存储
- **TuSDK** - 图像处理 SDK
- **网易云信 (Netease IM)** - 即时通讯
- **个推 (GeTui)** - 推送服务
- **友盟 (Umeng)** - 统计和日志
- **华为 HMS** - 华为移动服务
- **IJKPlayer** - 视频播放器
- **Glide** - 图片加载
- **Lucene** - 全文搜索

## 注意事项

dispatcher 请求带**签名 + AES 加密体**，且不同 channel 的报文结构不同：`api-user`
用 `secretKey` 加密、响应明文；`wohome` 用登录后的 `token` 加密、响应需用 `token` 解密。
细节和实测记录见 [dispatcher-protocol.md](dispatcher-protocol.md)。

由于梆梆加固的保护，完整的类结构和方法签名无法通过静态分析获取。可用的补充手段：
1. 网页版 bundle 逆向（本次确认接口的主要途径：`https://pan.wo.cn/js/*.js` 与内联 webpack runtime）
2. 在真实设备上使用 Frida 动态 dump（本目录 `dump_*.js`）
3. 使用 mitmproxy 抓包对照
