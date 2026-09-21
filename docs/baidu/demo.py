#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
baidu.py — 百度网盘 Python 客户端（仅标准库）

接口依据: docs/API_INTERFACES.md（com/baidu/sapi2/*、com/baidu/netdisk/cloudfile/io/CloudFileApi.java
          等 jadx 反编译证据）实现。

覆盖需求中的 8 个步骤:
    1. 登录            2. 打印文件列表    3. 创建目录        4. 再打印文件列表
    5. 重命名目录      6. 删除目录        7. 上传 test.txt   8. 下载 test.txt

用法示例:
    python3 baidu.py                                  # 用下方默认账号做账密登录，跑完整 8 步
    python3 baidu.py --bduss "<BDUSS>"                # 直接注入 BDUSS（推荐，最稳定）
    BAIDU_BDUSS="<BDUSS>" python3 baidu.py            # 同上，走环境变量
    python3 baidu.py --qr                             # 扫码登录（文字提示 + 保存二维码 PNG）
    python3 baidu.py --dir /我的资源 --limit 50       # 指定列表目录与条数

关于登录的实测结论（2026-09 实测，见 README 说明）:
  * passport.baidu.com/v2/sapi/login —— 文档 2.3 描述的端点。当前服务端对所有外部调用
    直接返回 {"errno":-3,"errMsg":"sapi appid error."}：appid/tpl 由 APK 运行时注入
    (SapiConfiguration.setProductLineInfo)，外部无法伪造，故此路已不可用。
  * passport.baidu.com/v2/api/?login、?getapi —— 已下线，返回 404。
  * wappass.baidu.com/wp/api/login —— 当前真实在用的账密入口。字段名 username/password，
    但服务端返回 50052（RESULT_CODE_NEW_HUMAN_VERIFY，需人机验证），无验证码无法自动完成。
  * 扫码登录（getqrcode / qrcode / qrbdusslogin）—— 已验证端点可用，需手机确认。
  * 直接注入 BDUSS —— 完全可用，脚本其余 7 步都依赖它。

因此脚本的登录优先级为: --bduss / BAIDU_BDUSS > --qr 扫码 > 账密登录（会走到人机验证并给出提示）。
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import http.cookiejar
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

# --------------------------------------------------------------------------------------
# 常量
# --------------------------------------------------------------------------------------

PAN_API = "https://pan.baidu.com/api/"                       # 文档 3.1 主 API
PCS_FILE = "https://d.pcs.baidu.com/rest/2.0/pcs/file"       # 文档 3.1 下载/上传节点
SAPI_LOGIN = "https://passport.baidu.com/v2/sapi/login"      # 文档 2.2 /v2/sapi/login
WAPPASS_LOGIN = "https://wappass.baidu.com/wp/api/login"     # 当前真实在用的账密入口

PCS_APP_ID = "250528"                                        # 文档 3.2 网盘 app_id
UA = "netdisk;9.15.1.32;Android;baidu.py"                    # 文档 1.4 / APK 9.15.1.32
REFERER = "https://pan.baidu.com/"

BLOCK_SIZE = 4 * 1024 * 1024                                 # 文档 5.4 分片上传粒度
TRUSTED_SUFFIXES = ("baidu.com", "baidupcs.com")             # 只向百度域发送凭证

# 文档 2.8 密码 RSA 公钥
RSA_EXPONENT = 0x10001
RSA_MODULUS = int(
    "B3C61EBBA4659C4CE3639287EE871F1F48F7930EA977991C7AFE3CC442FEA496"
    "43212E7D570C853F368065CC57A2014666DA8AE7D493FD47D171C0D894EEE3ED"
    "7F99F6798B7FFD7B5873227038AD23E3197631A8CB642213B9F27D4901AB0D92"
    "BFA27542AE890855396ED92775255C977F5C302F1E7ED4B1E369C12CB6B1822F",
    16,
)
RSA_BYTE_LEN = (RSA_MODULUS.bit_length() + 7) // 8

# 需求给定的测试账号（仅本地联调用；请勿把真实密码提交到版本库）
DEFAULT_USERNAME = "zh960721"
DEFAULT_PASSWORD = "jx960721"

DEMO_DIR = "/baidu_py_demo"              # 第 3 步创建、第 5 步改名、第 6 步删除
DEMO_DIR_RENAMED = "/baidu_py_demo_renamed"
DEMO_FILE_REMOTE = "/test.txt"           # 第 7 步上传的远端路径
DEMO_FILE_LOCAL = "test.txt"             # 第 7 步的本地上传源
DEMO_FILE_DOWNLOADED = "test_downloaded.txt"   # 第 8 步的本地落地文件


class BaiduError(RuntimeError):
    """百度接口返回的业务错误。"""

    def __init__(self, message: str, errno: int | None = None):
        super().__init__(message)
        self.errno = errno


# --------------------------------------------------------------------------------------
# 工具函数
# --------------------------------------------------------------------------------------

def rsa_encrypt_hex(plaintext: str, base64_encode: bool) -> str:
    """文档 2.8: 密码先 Base64 再 RSA；用户名只做 RSA。

    对应 PwdRSAEncryptor.encryptString(preEncode, text)，返回定长大端十六进制串。
    """
    data = base64.b64encode(plaintext.encode()).decode() if base64_encode else plaintext
    cipher = pow(int.from_bytes(data.encode(), "big"), RSA_EXPONENT, RSA_MODULUS)
    return cipher.to_bytes(RSA_BYTE_LEN, "big").hex()


def md5_hex(data: bytes) -> str:
    return hashlib.md5(data).hexdigest()


def bdstoken_of(bduss: str) -> str:
    """文档 1.2: bdstoken = MD5(BDUSS).toLowerCase()"""
    return md5_hex(bduss.encode()).lower()


def file_md5(path: str, chunk: int = 1 << 20) -> str:
    digest = hashlib.md5()
    with open(path, "rb") as fh:
        for block in iter(lambda: fh.read(chunk), b""):
            digest.update(block)
    return digest.hexdigest()


def check_trusted(url: str) -> str:
    """只允许 https 的百度域，避免把 BDUSS 发到第三方节点。"""
    parsed = urllib.parse.urlsplit(url)
    host = parsed.hostname or ""
    if parsed.scheme != "https" or not any(
        host == suffix or host.endswith("." + suffix) for suffix in TRUSTED_SUFFIXES
    ):
        raise BaiduError(f"拒绝向非百度域发送凭证: {url}")
    return url


def human_size(num: int) -> str:
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if num < 1024 or unit == "TB":
            return f"{num:.0f}{unit}" if unit == "B" else f"{num:.1f}{unit}"
        num /= 1024.0
    return f"{num}B"


def parse_credentials(raw: str, stoken: str = "") -> tuple[str, str]:
    """接受裸 BDUSS、`BDUSS=xxx; STOKEN=yyy` 或整段 Cookie。"""
    raw = raw.strip().removeprefix("Cookie:").strip()
    values: dict[str, str] = {}
    if "BDUSS=" in raw:
        for pair in raw.split(";"):
            if "=" not in pair:
                continue
            key, _, value = pair.partition("=")
            if key.strip() in ("BDUSS", "STOKEN", "panPSC"):
                values[key.strip()] = value.strip()
    else:
        values["BDUSS"] = raw
    if stoken.strip():
        values["STOKEN"] = stoken.strip()
    if not values.get("BDUSS"):
        raise BaiduError("Cookie 中缺少 BDUSS。")
    return values["BDUSS"], values.get("STOKEN", "")


# --------------------------------------------------------------------------------------
# 客户端
# --------------------------------------------------------------------------------------

class BaiduPan:
    """百度网盘 API 客户端。凭证来自 BDUSS Cookie + bdstoken 查询参数（文档 1.1~1.3）。"""

    def __init__(self, bduss: str, stoken: str = "", timeout: int = 60):
        self.bduss = bduss
        self.stoken = stoken
        self.bdstoken = bdstoken_of(bduss)
        self.timeout = timeout
        # 网盘 API 请求统一显式携带 Cookie；不挂 CookieJar 以免与登录会话串味
        self._opener = urllib.request.build_opener()

    # -- 基础请求 ----------------------------------------------------------------------

    @property
    def cookie(self) -> str:
        parts = [f"BDUSS={self.bduss}"]
        if self.stoken:
            parts.append(f"STOKEN={self.stoken}")
        return "; ".join(parts)

    def _open(self, request: urllib.request.Request):
        request.add_header("User-Agent", UA)
        request.add_header("Referer", REFERER)
        request.add_header("Cookie", self.cookie)
        return self._opener.open(request, timeout=self.timeout)

    def get(self, url: str, params: dict | None = None) -> bytes:
        if params:
            url = url + ("&" if "?" in url else "?") + urllib.parse.urlencode(params)
        with self._open(urllib.request.Request(url)) as resp:
            return resp.read()

    def post_form(self, url: str, params: dict | None = None, form: dict | None = None) -> bytes:
        if params:
            url = url + ("&" if "?" in url else "?") + urllib.parse.urlencode(params)
        body = urllib.parse.urlencode(form or {}).encode()
        req = urllib.request.Request(url, data=body)
        req.add_header("Content-Type", "application/x-www-form-urlencoded")
        with self._open(req) as resp:
            return resp.read()

    def post_multipart(self, url: str, params: dict, field: str, filename: str, content: bytes) -> bytes:
        boundary = "----baidupy" + uuid.uuid4().hex
        body = b"".join([
            f"--{boundary}\r\n".encode(),
            f'Content-Disposition: form-data; name="{field}"; filename="{filename}"\r\n'.encode(),
            b"Content-Type: application/octet-stream\r\n\r\n",
            content,
            f"\r\n--{boundary}--\r\n".encode(),
        ])
        url = url + ("&" if "?" in url else "?") + urllib.parse.urlencode(params)
        req = urllib.request.Request(url, data=body)
        req.add_header("Content-Type", f"multipart/form-data; boundary={boundary}")
        with self._open(req) as resp:
            return resp.read()

    def api(self, endpoint: str, params: dict | None = None, form: dict | None = None) -> dict:
        """调用 /api/<endpoint>，自动注入 bdstoken / app_id（文档 1.2）。

        注意：不要额外传 clienttype。实测 /api/list 带 clienttype=1 会返回 errno=1，
        去掉后正常；文档 8.1 的 /api/list 参数表里也没有这个字段。
        """
        query = {
            "bdstoken": self.bdstoken,
            "app_id": PCS_APP_ID,
            **(params or {}),
        }
        raw = self.post_form(PAN_API + endpoint, query, form) if form is not None \
            else self.get(PAN_API + endpoint, query)
        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            raise BaiduError(f"{endpoint} 未返回 JSON，可能需要重新登录或完成安全验证: {raw[:200]!r}")
        return self._check(endpoint, data)

    @staticmethod
    def _check(endpoint: str, data: dict) -> dict:
        code = int(data.get("errno", 0) or 0)
        if code != 0:
            messages = {
                -6: "登录凭证已失效，请重新登录。",
                -7: "文件名不合法或无权操作。",
                -8: "同名文件或文件夹已存在。",
                -9: "文件或文件夹不存在。",
                -19: "百度要求输入验证码。",
                132: "百度要求安全验证。",
            }
            detail = data.get("show_msg") or data.get("errmsg") or data.get("error_msg") or "未知错误"
            raise BaiduError(f"{endpoint} 失败 (errno={code}): {messages.get(code, str(detail)[:200])}", code)
        return data

    # -- 文件管理 (文档 6 / 8 / 10) ------------------------------------------------------

    def list_dir(self, directory: str = "/", start: int = 0, limit: int = 100,
                 order: str = "name", desc: int = 0) -> dict:
        """文档 8: GET /api/list"""
        # 文档 8.1 列出了 isdir 参数但未说明语义；项目内已验证的 web-demo/baidu-api.mjs
        # 不传该参数，这里保持一致，避免漏掉目录或文件。
        data = self.api("list", {
            "dir": directory, "start": start, "limit": limit,
            "order": order, "desc": desc, "preset": 0,
        })
        if not isinstance(data.get("list"), list):
            raise BaiduError("列表响应缺少 list 字段。")
        return data

    def mkdir(self, path: str) -> dict:
        """文档 10: POST /api/create?a=commit"""
        now = int(time.time())
        return self.api("create", {"a": "commit"}, {
            "path": path, "size": 0, "isdir": 1,
            "local_ctime": now, "local_mtime": now, "block_list": "[]",
        })

    def rename(self, path: str, newname: str) -> dict:
        """文档 6.2: POST /api/filemanager?opera=rename

        文档写 async=2（异步）；这里用 async=0 以同步拿到逐项结果，便于脚本判定成败。
        """
        data = self.api("filemanager", {"opera": "rename", "async": 0, "ondup": 1},
                        {"filelist": json.dumps([{"path": path, "newname": newname}])})
        if int(data.get("taskid", 0) or 0) <= 0 and not data.get("info"):
            raise BaiduError(f"重命名未生效: {data}")
        return data

    def delete(self, paths: list[str]) -> dict:
        """文档 6.5: POST /api/filemanager?opera=delete，filelist 为路径字符串数组。"""
        data = self.api("filemanager", {"opera": "delete", "async": 0, "ondup": 1},
                        {"filelist": json.dumps(paths)})
        ok_codes = {0, -9, 31066}      # 文档 6.5 解析逻辑：这三种都视为删除成功
        failures = [item for item in data.get("info", [])
                    if int(item.get("errno", 0) or 0) not in ok_codes]
        if failures:
            raise BaiduError(f"删除失败: {failures}")
        return data

    # -- 上传 (文档 5) ------------------------------------------------------------------

    def upload(self, local_path: str, remote_path: str) -> dict:
        """PreCreate → LocateUpload → SuperFile2 → Create"""
        size = os.path.getsize(local_path)
        blocks: list[str] = []
        with open(local_path, "rb") as fh:
            while True:
                chunk = fh.read(BLOCK_SIZE)
                if not chunk:
                    break
                blocks.append(md5_hex(chunk))
        if not blocks:
            blocks.append(md5_hex(b""))

        form = {
            "path": remote_path, "size": size, "isdir": 0,
            # 文档 5.2: rtype 0=重命名 / 1=覆盖 / 2=跳过。
            # 用 1（覆盖）而非 0：实测 rtype=0 时若目标已存在，create 阶段直接返回
            # errno=-8，脚本无法重跑；覆盖语义也更贴合「上传测试文件」。
            "block_list": json.dumps(blocks), "rtype": 1,
        }
        pre = self.api("precreate", None, {**form, "autoinit": 1,
                                           "content-md5": file_md5(local_path)})
        if int(pre.get("return_type", 0) or 0) == 2:
            return {**pre, "rapid": True}         # 秒传：服务端已有该文件

        uploadid = pre.get("uploadid")
        need = [int(i) for i in pre.get("block_list", [])]
        if not uploadid or any(i < 0 or i >= len(blocks) for i in need):
            raise BaiduError(f"预创建响应异常: {pre}")

        if need:
            located = json.loads(self.get(PCS_FILE, {
                "method": "locateupload", "upload_version": "2.0",
                "app_id": PCS_APP_ID,
                **({"uploadsign": pre["uploadsign"]} if pre.get("uploadsign") else {}),
            }))
            servers = located.get("servers") or []
            if not servers:
                raise BaiduError(f"未取得上传节点: {located}")
            base = check_trusted(servers[0]["server"])

            with open(local_path, "rb") as fh:
                for index in need:
                    fh.seek(index * BLOCK_SIZE)
                    chunk = fh.read(BLOCK_SIZE)
                    result = json.loads(self.post_multipart(
                        base + "/rest/2.0/pcs/superfile2",
                        {"method": "upload", "type": "tmpfile", "path": remote_path,
                         "partoffset": index * BLOCK_SIZE, "app_id": PCS_APP_ID,
                         "uploadid": uploadid, "partseq": index},
                        "file", "chunk", chunk,
                    ))
                    if result.get("md5") and result["md5"] != blocks[index]:
                        raise BaiduError(f"分片 {index} 校验失败。")

        created = self.api("create", None, {**form, "uploadid": uploadid})
        return {**created, "rapid": False}

    # -- 下载 (文档 4) ------------------------------------------------------------------

    def locate_download(self, remote_path: str) -> str:
        """文档 4.1: GET /rest/2.0/pcs/file?method=locatedownload"""
        raw = self.get(PCS_FILE, {
            "method": "locatedownload", "path": remote_path, "ver": "2.0",
            "dtype": 0, "esl": 1, "ehps": 1, "app_id": PCS_APP_ID, "check_blue": 1,
        })
        data = json.loads(raw)
        urls = data.get("urls") or []
        if not urls:
            raise BaiduError(f"未取得下载地址: {raw[:200]!r}")
        return check_trusted(urls[0]["url"])

    def download(self, remote_path: str, local_path: str) -> int:
        url = self.locate_download(remote_path)
        request = urllib.request.Request(url)
        with self._open(request) as resp, open(local_path, "wb") as out:
            total = 0
            while True:
                chunk = resp.read(1 << 16)
                if not chunk:
                    break
                out.write(chunk)
                total += len(chunk)
        return total


# --------------------------------------------------------------------------------------
# 登录
# --------------------------------------------------------------------------------------

def _login_opener() -> urllib.request.OpenerDirector:
    """登录专用 opener：保留服务端 Set-Cookie（BAIDUID 等，文档 2.13）。"""
    jar = http.cookiejar.CookieJar()
    return urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))


def login_with_password(username: str, password: str) -> tuple[str, str]:
    """账密登录。先按文档 2.3 走 sapi，再回退到当前在用的 wappass 入口。

    返回 (bduss, stoken)。两者当前都会被服务端风控拦下并抛出 BaiduError —— 这是服务端
    行为，不是脚本缺陷，异常信息里会说明下一步该怎么做。
    """
    encrypted_user = rsa_encrypt_hex(username, base64_encode=False)
    encrypted_pwd = rsa_encrypt_hex(password, base64_encode=True)

    # --- 路径 A：文档 2.3 的 /v2/sapi/login -------------------------------------------
    params = {
        "username": encrypted_user, "password": encrypted_pwd,
        "isphone": "0", "isEncrypted": "1", "encryptedId": "2",
        "encryptedType": "rsa", "alg": "rsa", "countrycode": "86",
        "loginmerge": "1", "logLoginType": "android",
        "t": str(int(time.time() * 1000)), "time": str(int(time.time() * 1000)),
        "uid": "", "cv": "9.15.1.32", "lang": "zh", "adapter": "3",
        "tpl": "netdisk", "apiver": "v3", "appid": "200019",
        "staticpage": "https://pan.baidu.com/res/static/thirdparty/loginResult.html",
        "charset": "UTF-8", "u": "https://pan.baidu.com/", "papi": "1", "sms": "0",
    }
    sapi_note = ""
    try:
        request = urllib.request.Request(
            SAPI_LOGIN, data=urllib.parse.urlencode(params).encode(),
            headers={"User-Agent": UA, "Content-Type": "application/x-www-form-urlencoded",
                     "Referer": REFERER})
        with _login_opener().open(request, timeout=25) as resp:
            payload = json.loads(resp.read().decode() or "{}")
        if int(payload.get("errno", 0) or 0) in (0, -100) and payload.get("bduss"):
            return payload["bduss"], payload.get("stoken", "")
        sapi_note = f"sapi/login 返回 errno={payload.get('errno')} {payload.get('errMsg', '')}".strip()
    except Exception as exc:                                  # noqa: BLE001 - 回退到下一条路径
        sapi_note = f"sapi/login 请求异常: {exc}"

    # --- 路径 B：当前在用的 wappass 账密入口 -------------------------------------------
    form = {"username": username, "password": encrypted_pwd, "tpl": "netdisk",
            "appid": "200019", "isEncrypted": "1", "encryptedType": "rsa",
            "encryptedId": "2", "alg": "rsa", "staticpage": "https://pan.baidu.com/",
            "charset": "UTF-8", "tt": str(int(time.time() * 1000))}
    try:
        opener = _login_opener()
        opener.open(urllib.request.Request("https://wappass.baidu.com/passport/",
                                           headers={"User-Agent": UA}), timeout=20).read()
        request = urllib.request.Request(
            WAPPASS_LOGIN, data=urllib.parse.urlencode(form).encode(),
            headers={"User-Agent": UA, "Content-Type": "application/x-www-form-urlencoded",
                     "Referer": "https://wappass.baidu.com/passport/"})
        with opener.open(request, timeout=25) as resp:
            body = resp.read().decode(errors="replace")
        payload = json.loads(body)
        info = payload.get("errInfo", {})
        code, message = str(info.get("no", "")), str(info.get("msg", ""))
        data = payload.get("data", {})
        if data.get("bduss"):
            return data["bduss"], data.get("ptoken", "") or data.get("stoken", "")
        if code == "50052":
            raise BaiduError(
                "账密登录被百度风控拦截：服务端要求人机验证 (50052 系统繁忙/需人机校验)，"
                "脚本无法自动通过。请改用 `--qr` 扫码登录，或用 `--bduss` 直接注入 BDUSS。")
        raise BaiduError(f"账密登录失败: {message or body[:200]}")
    except BaiduError:
        raise
    except Exception as exc:                                  # noqa: BLE001
        raise BaiduError(f"账密登录失败。\n  {sapi_note}\n  wappass/login 异常: {exc}") from exc


def login_with_qrcode(qr_path: str = "baidu_login_qrcode.png", timeout_sec: int = 180) -> tuple[str, str]:
    """扫码登录（文档 2.5）。

    实测可用端点:
      GET /v2/api/getqrcode?lp=pc&qrloginfrom=pc     -> {"sign": ..., "imgurl": ...}
      GET https://<imgurl>                            -> 二维码 PNG
      GET /v3/login/main/qrbdusslogin?sign=<sign>     -> 轮询登录结果

    注意: 轮询成功时的响应字段格式未经真实扫码验证（没有可用的手机确认），
    因此这里对常见字段做了兼容解析，并在失败时打印原始响应，便于按实际返回调整。
    """
    opener = _login_opener()
    opener.open(urllib.request.Request("https://passport.baidu.com/",
                                       headers={"User-Agent": UA}), timeout=20).read()
    meta = json.loads(opener.open(urllib.request.Request(
        "https://passport.baidu.com/v2/api/getqrcode?lp=pc&qrloginfrom=pc",
        headers={"User-Agent": UA, "Referer": REFERER}), timeout=20).read())
    sign = meta.get("sign")
    if not sign:
        raise BaiduError(f"未取得扫码 sign: {meta}")
    with opener.open(urllib.request.Request("https://" + meta["imgurl"],
                                            headers={"User-Agent": UA}), timeout=20) as resp:
        with open(qr_path, "wb") as fh:
            fh.write(resp.read())
    print(f"  二维码已保存到 {os.path.abspath(qr_path)}，请用「百度网盘 App」扫码并确认。")

    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        raw = opener.open(urllib.request.Request(
            f"https://passport.baidu.com/v3/login/main/qrbdusslogin?sign={urllib.parse.quote(sign)}&lp=pc",
            headers={"User-Agent": UA, "Referer": REFERER}), timeout=20).read().decode(errors="replace")
        try:
            payload = json.loads(raw)
        except json.JSONDecodeError:
            payload = {}
        data = payload.get("data") if isinstance(payload.get("data"), dict) else payload
        bduss = (data or {}).get("bduss")
        if bduss:
            return bduss, (data or {}).get("stoken", "") or (data or {}).get("ptoken", "")
        time.sleep(3)
    raise BaiduError("扫码登录超时。")


# --------------------------------------------------------------------------------------
# 8 步演示流程
# --------------------------------------------------------------------------------------

def step(title: str) -> None:
    print(f"\n{'=' * 68}\n{title}\n{'=' * 68}")


def show_list(pan: BaiduPan, directory: str, limit: int) -> None:
    data = pan.list_dir(directory, limit=limit)
    items = data["list"]
    if not items:
        print(f"  {directory} 目录为空")
        return
    for item in items:
        kind = "DIR " if int(item.get("isdir", 0)) else "FILE"
        mark = "/" if kind == "DIR " else ""
        size = "" if kind == "DIR " else f"  {human_size(int(item.get('size', 0)))}"
        print(f"  [{kind}] {item.get('path', item.get('server_filename', '?'))}{mark}{size}")
    if data.get("has_more"):
        print("  …（还有更多，用 --limit 调整）")


def run_demo(pan: BaiduPan, directory: str, limit: int) -> None:
    step("2. 打印文件列表")
    show_list(pan, directory, limit)

    step(f"3. 创建目录 {DEMO_DIR}")
    pan.mkdir(DEMO_DIR)
    print(f"  已创建 {DEMO_DIR}")

    step("4. 再次打印文件列表")
    show_list(pan, directory, limit)

    step(f"5. 重命名 {DEMO_DIR} -> {DEMO_DIR_RENAMED}")
    pan.rename(DEMO_DIR, DEMO_DIR_RENAMED.lstrip("/"))
    print("  重命名完成")

    step(f"6. 删除 {DEMO_DIR_RENAMED}")
    pan.delete([DEMO_DIR_RENAMED])
    print("  删除完成")

    step(f"7. 上传 {DEMO_FILE_LOCAL} -> {DEMO_FILE_REMOTE}")
    if not os.path.exists(DEMO_FILE_LOCAL):
        with open(DEMO_FILE_LOCAL, "w", encoding="utf-8") as fh:
            fh.write(f"baidu.py upload test\n生成时间: {time.strftime('%Y-%m-%d %H:%M:%S')}\n")
        print(f"  已生成本地 {DEMO_FILE_LOCAL}")
    local_md5 = file_md5(DEMO_FILE_LOCAL)
    result = pan.upload(DEMO_FILE_LOCAL, DEMO_FILE_REMOTE)
    print(f"  上传完成{'（秒传）' if result.get('rapid') else ''}，本地 MD5={local_md5}")

    step(f"8. 下载 {DEMO_FILE_REMOTE} -> {DEMO_FILE_DOWNLOADED}")
    size = pan.download(DEMO_FILE_REMOTE, DEMO_FILE_DOWNLOADED)
    remote_md5 = file_md5(DEMO_FILE_DOWNLOADED)
    print(f"  已下载 {human_size(size)} 到 {DEMO_FILE_DOWNLOADED}")
    print(f"  校验: {'一致 ✓' if remote_md5 == local_md5 else f'不一致 ✗ ({local_md5} vs {remote_md5})'}")


def main(argv: list[str] | None = None) -> int:
    try:                                   # 管道重定向时也保持与 stderr 的先后顺序
        sys.stdout.reconfigure(line_buffering=True)
    except (AttributeError, ValueError):
        pass
    parser = argparse.ArgumentParser(
        description="百度网盘客户端：登录 / 列目录 / 建目录 / 改名 / 删除 / 上传 / 下载")
    parser.add_argument("--username", default=os.getenv("BAIDU_USERNAME", DEFAULT_USERNAME))
    parser.add_argument("--password", default=os.getenv("BAIDU_PASSWORD", DEFAULT_PASSWORD))
    parser.add_argument("--bduss", default=os.getenv("BAIDU_BDUSS", ""),
                        help="直接注入 BDUSS，跳过账密登录（推荐）")
    parser.add_argument("--stoken", default=os.getenv("BAIDU_STOKEN", ""))
    parser.add_argument("--qr", action="store_true", help="扫码登录")
    parser.add_argument("--dir", default="/", help="第 2/4 步列出的目录")
    parser.add_argument("--limit", type=int, default=50)
    args = parser.parse_args(argv)

    step("1. 登录")
    try:
        if args.bduss:
            bduss, stoken = parse_credentials(args.bduss, args.stoken)
            print(f"  使用注入的 BDUSS ({bduss[:6]}…)，bdstoken={bdstoken_of(bduss)}")
        elif args.qr:
            bduss, stoken = login_with_qrcode()
            print("  扫码登录成功。")
        else:
            print(f"  账密登录中…（账号 {args.username}）")
            bduss, stoken = login_with_password(args.username, args.password)
            print("  登录成功。")
    except BaiduError as exc:
        print(f"\n登录失败：{exc}", file=sys.stderr)
        return 2

    pan = BaiduPan(bduss, stoken)
    try:
        run_demo(pan, args.dir, args.limit)
    except urllib.error.HTTPError as exc:
        print(f"\nHTTP 错误 {exc.code}: {exc.read()[:300]!r}", file=sys.stderr)
        return 1
    except BaiduError as exc:
        print(f"\n操作失败：{exc}", file=sys.stderr)
        return 1
    print("\n全部步骤完成。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
