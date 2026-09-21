#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
联通云盘 (WoCloud / pan.wo.cn) 文件操作脚本

协议来自 pan.wo.cn 网页版 bundle 的逆向 + 实测验证（见 docs/dispatcher-protocol.md）。

流程
----
1. 登录     PcWebLogin(图形验证码) -> 短信验证码 -> PcLoginVerifyCode -> access_token
2. 列表     QueryAllFiles
3. 上传     POST {uploadHost}/openapi/client/upload2C (multipart)
4. 下载     GetDownloadUrl -> downloadUrl -> GET
5. 重命名   RenameFileOrDirectory
6. 删除     DeleteFile

dispatcher 协议
---------------
    POST https://panservice.mail.wo.cn/{api-user|wohome}/dispatcher
    header: {key: 操作名, resTime, reqSeq, channel, sign, version}
            sign = md5(key + resTime + reqSeq + channel + version)   # 小写 hex
    api-user: body = {param: AES(参数, SECRET_KEY), clientId, secret:true}   # 响应明文
    wohome  : body = {param: AES(参数+clientId, TOKEN), key:true}            # 响应用 TOKEN 解密

    AES-128-CBC / PKCS7，密钥取前 16 字节，IV = "wNSOYIB1k1DjY5lA"

依赖: pycryptodome   （pip install pycryptodome；验证码自动识别额外需要 macOS + pillow）
"""

import base64
import hashlib
import json
import os
import random
import ssl
import string
import sys
import time
import urllib.error
import urllib.request
import uuid as uuidlib

try:
    from Crypto.Cipher import AES
    from Crypto.Util.Padding import pad, unpad
except ImportError:
    sys.exit("缺少依赖，请先安装：pip install pycryptodome")

# ----------------------------------------------------------------------------
# 配置
# ----------------------------------------------------------------------------
ACCOUNT = os.environ.get("WO_ACCOUNT", "18566949504")
PASSWORD = os.environ.get("WO_PASSWORD", "Jx960721!")
TOKEN = os.environ.get("WO_TOKEN", "")   # 提供则跳过登录（复用已有 token）

API_HOST = "https://panservice.mail.wo.cn"
CLIENT_ID = "1001000021"
SECRET_KEY = "XFmi9GS2hzk98jGX"
AES_IV = "wNSOYIB1k1DjY5lA"
DEFAULT_UPLOAD_HOST = "https://hyupload.pan.wo.cn"   # 实际由 GetZoneInfo 下发

SPACE_TYPE = "0"      # 0=个人云 1=家庭云 4=保密空间
ROOT_DIR_ID = "0"
CAPTCHA_PNG = "/tmp/wocloud_captcha.png"
TEST_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "test.txt")
DOWNLOAD_TO = os.path.join(os.path.dirname(os.path.abspath(__file__)), "test_downloaded.txt")

_CTX = ssl.create_default_context()
_CTX.check_hostname = False
_CTX.verify_mode = ssl.CERT_NONE


# ----------------------------------------------------------------------------
# 加密 / 签名
# ----------------------------------------------------------------------------
def aes_encrypt(plain: str, key: str, iv: str = AES_IV) -> str:
    """AES-128-CBC + PKCS7，返回 base64（等价 CryptoJS AES.encrypt(...).toString()）"""
    cipher = AES.new(key[:16].encode(), AES.MODE_CBC, iv.encode())
    return base64.b64encode(cipher.encrypt(pad(plain.encode(), 16))).decode()


def aes_decrypt(b64: str, key: str, iv: str = AES_IV) -> str:
    cipher = AES.new(key[:16].encode(), AES.MODE_CBC, iv.encode())
    return unpad(cipher.decrypt(base64.b64decode(b64)), 16).decode()


def make_sign(op, res_time, req_seq, channel, version=""):
    return hashlib.md5(f"{op}{res_time}{req_seq}{channel}{version}".encode()).hexdigest()


def rand_str(n):
    return "".join(random.choices(string.ascii_letters + string.digits, k=n))


# ----------------------------------------------------------------------------
# 客户端
# ----------------------------------------------------------------------------
class WoCloud:
    def __init__(self, account=ACCOUNT, password=PASSWORD):
        self.account = account
        self.password = password
        self.token = ""
        self.space_type = SPACE_TYPE
        self.upload_host = DEFAULT_UPLOAD_HOST

    # -- HTTP -------------------------------------------------------------
    def _post_json(self, url, obj, token=""):
        data = json.dumps(obj, ensure_ascii=False).encode()
        req = urllib.request.Request(url, data=data, method="POST")
        req.add_header("Content-Type", "application/json")
        req.add_header("accesstoken", token or "")
        req.add_header("User-Agent", "Mozilla/5.0 WoCloud-Py/1.0")
        try:
            with urllib.request.urlopen(req, timeout=30, context=_CTX) as r:
                return json.loads(r.read().decode("utf-8", "replace"))
        except urllib.error.HTTPError as e:
            return {"STATUS": str(e.code), "RSP": {"RSP_CODE": "-1", "RSP_DESC": e.read().decode()[:200]}}
        except Exception as e:
            return {"STATUS": "-1", "RSP": {"RSP_CODE": "-1", "RSP_DESC": f"网络异常: {e}"}}

    def _post_multipart(self, url, fields, file_field, file_name, file_bytes):
        boundary = "----WoCloudPy" + uuidlib.uuid4().hex
        body = bytearray()

        def w(x):
            body.extend(x.encode() if isinstance(x, str) else x)

        for k, v in fields.items():
            w(f"--{boundary}\r\n")
            w(f'Content-Disposition: form-data; name="{k}"\r\n\r\n')
            w(f"{v}\r\n")
        w(f"--{boundary}\r\n")
        w(f'Content-Disposition: form-data; name="{file_field}"; filename="{file_name}"\r\n')
        w("Content-Type: application/octet-stream\r\n\r\n")
        w(file_bytes)
        w(f"\r\n--{boundary}--\r\n")

        req = urllib.request.Request(url, data=bytes(body), method="POST")
        req.add_header("Content-Type", f"multipart/form-data; boundary={boundary}")
        req.add_header("accesstoken", self.token or "")
        try:
            with urllib.request.urlopen(req, timeout=180, context=_CTX) as r:
                return json.loads(r.read().decode("utf-8", "replace"))
        except urllib.error.HTTPError as e:
            return {"code": str(e.code), "msg": e.read().decode()[:200]}
        except Exception as e:
            return {"code": "-1", "msg": f"网络异常: {e}"}

    # -- dispatcher -------------------------------------------------------
    def dispatch(self, op, params=None, channel="wohome"):
        params = params or {}
        res_time = int(time.time() * 1000)
        req_seq = random.randint(100000, 189999)
        if channel == "api-user":
            payload = json.dumps(params, ensure_ascii=False, separators=(",", ":"))
            body = {"param": aes_encrypt(payload, SECRET_KEY), "clientId": CLIENT_ID, "secret": True}
            resp_key = None
        else:
            signed = dict(params)
            signed["clientId"] = CLIENT_ID
            payload = json.dumps(signed, ensure_ascii=False, separators=(",", ":"))
            resp_key = self.token or SECRET_KEY
            body = {"param": aes_encrypt(payload, resp_key), "key": True}
        obj = {"header": {"key": op, "resTime": res_time, "reqSeq": req_seq, "channel": channel,
                          "sign": make_sign(op, res_time, req_seq, channel), "version": ""},
               "body": body}
        resp = self._post_json(f"{API_HOST}/{channel}/dispatcher", obj, token=self.token)
        return self._decrypt_resp(resp, resp_key)

    @staticmethod
    def _decrypt_resp(resp, key):
        if not key or not isinstance(resp, dict):
            return resp
        data = (resp.get("RSP") or {}).get("DATA")
        if isinstance(data, str) and data:
            try:
                resp["RSP"]["DATA"] = json.loads(aes_decrypt(data, key))
            except Exception:
                pass
        return resp

    @staticmethod
    def _ok(resp):
        return isinstance(resp, dict) and (resp.get("RSP") or {}).get("RSP_CODE") == "0000"

    @staticmethod
    def _desc(resp):
        rsp = (resp or {}).get("RSP", {}) if isinstance(resp, dict) else {}
        return f'{rsp.get("RSP_CODE")} {rsp.get("RSP_DESC")}'

    # -- 1. 登录 ----------------------------------------------------------
    def fetch_captcha(self, uuid_str, save_path=CAPTCHA_PNG):
        url = f"{API_HOST}/api-user/getverifycode?uuid={uuid_str}"
        try:
            with urllib.request.urlopen(urllib.request.Request(url), timeout=20, context=_CTX) as r, \
                    open(save_path, "wb") as out:
                out.write(r.read())
            print(f"    图形验证码已保存: {save_path}")
            return True
        except Exception as e:
            print(f"    [!] 获取图形验证码失败: {e}")
            return False

    @staticmethod
    def ocr_captcha(image_path):
        """macOS 上用 Vision 框架识别验证码；失败返回 None"""
        import shutil
        import subprocess
        if not shutil.which("swift"):
            return None
        swift_src = (
            "import Foundation\nimport Vision\nimport AppKit\n"
            "let p = CommandLine.arguments[1]\n"
            "guard let i = NSImage(contentsOfFile: p), let c = i.cgImage(forProposedRect: nil, context: nil, hints: nil) else { exit(1) }\n"
            "let r = VNRecognizeTextRequest { q, _ in\n"
            "  guard let o = q.results as? [VNRecognizedTextObservation] else { return }\n"
            "  for x in o { if let t = x.topCandidates(1).first { print(t.string) } }\n}\n"
            "r.recognitionLevel = .accurate\nr.usesLanguageCorrection = false\n"
            "try? VNImageRequestHandler(cgImage: c, options: [:]).perform([r])\n"
        )
        target = image_path
        try:
            from PIL import Image, ImageOps
            im = Image.open(image_path).convert("L")
            im = ImageOps.autocontrast(im.resize((im.width * 8, im.height * 8), Image.LANCZOS))
            im.save("/tmp/_cap_ocr.png")
            target = "/tmp/_cap_ocr.png"
        except Exception:
            pass
        try:
            with open("/tmp/_captcha_ocr.swift", "w") as fh:
                fh.write(swift_src)
            out = subprocess.run(["swift", "/tmp/_captcha_ocr.swift", target],
                                 capture_output=True, text=True, timeout=90).stdout.strip()
            return out.replace(" ", "") or None
        except Exception:
            return None

    def login(self):
        """密码登录：图形验证码 -> 短信验证码 -> access_token（后两步需人工输入）"""
        if TOKEN:
            self.token = TOKEN
            print(f"[1] 使用 WO_TOKEN 跳过登录 (token={TOKEN[:16]}...)")
            return True
        print(f"[1] 登录 account={self.account}")
        uuid_str = str(uuidlib.uuid4())
        params = {"phone": self.account, "password": self.password, "uuid": uuid_str,
                  "verifyCode": "", "clientSecret": SECRET_KEY}
        resp = self.dispatch("PcWebLogin", params, channel="api-user")
        code = resp.get("RSP", {}).get("RSP_CODE")
        data = resp.get("RSP", {}).get("DATA") or {}
        print(f"    PcWebLogin -> {self._desc(resp)}")

        if code == "6006" or data.get("isNeedVerfyCode") == 1:
            if self.fetch_captcha(uuid_str):
                guess = self.ocr_captcha(CAPTCHA_PNG)
                if guess:
                    print(f"    自动识别结果: {guess}")
                params["verifyCode"] = input(f"    图形验证码[回车采用 {guess}]: ").strip() or (guess or "")
                resp = self.dispatch("PcWebLogin", params, channel="api-user")
                code = resp.get("RSP", {}).get("RSP_CODE")
                data = resp.get("RSP", {}).get("DATA") or {}
                print(f"    PcWebLogin(带图形验证码) -> {self._desc(resp)}")

        if code == "0000" and data.get("access_token"):
            self.token = data["access_token"]
        elif code in ("0000", "6008"):
            print("    服务端已发送短信验证码")
            sms = input("    短信验证码: ").strip()
            vp = {"phone": self.account, "password": self.password, "uuid": uuid_str,
                  "verifyCode": params.get("verifyCode", ""), "messageCode": sms,
                  "clientSecret": SECRET_KEY}
            vresp = self.dispatch("PcLoginVerifyCode", vp, channel="api-user")
            print(f"    PcLoginVerifyCode -> {self._desc(vresp)}")
            if self._ok(vresp):
                self.token = (vresp["RSP"].get("DATA") or {}).get("access_token", "")
        else:
            print(f"    [!] 登录失败：{self._desc(resp)}")
            return False

        if not self.token:
            print("    [!] 未取得 token")
            return False
        print(f"    token = {self.token[:24]}...")
        return True

    # -- 2. 文件列表 ------------------------------------------------------
    def list_files(self, parent_dir_id=ROOT_DIR_ID, page_num=0, page_size=50):
        """注意：pageNum 从 0 开始；返回项用 name/size 字段"""
        print(f"[2] 获取文件列表 directoryId={parent_dir_id}")
        params = {"spaceType": self.space_type, "parentDirectoryId": parent_dir_id,
                  "pageNum": page_num, "pageSize": page_size, "sortRule": "1"}
        resp = self.dispatch("QueryAllFiles", params)
        if not self._ok(resp):
            print(f"    [!] 失败：{self._desc(resp)}")
            return []
        data = resp["RSP"].get("DATA") or {}
        files = data.get("files") or []
        dirs = data.get("systemDirs") or data.get("directorys") or []
        print(f"    共 {len(files)} 个文件, {len(dirs)} 个目录")
        for f in files:
            print(f"      - {f.get('name')}  id={f.get('id')}  size={f.get('size')}")
        return files

    def find_file(self, name, parent_dir_id=ROOT_DIR_ID):
        return next((f for f in self.list_files(parent_dir_id) if f.get("name") == name), None)

    # -- 3. 上传 ----------------------------------------------------------
    def get_upload_host(self):
        resp = self.dispatch("GetZoneInfo", {"appId": "10000001"})
        if self._ok(resp):
            url = (resp["RSP"].get("DATA") or {}).get("url")
            if url:
                self.upload_host = url.rstrip("/")
        return self.upload_host

    def upload(self, local_path, remote_name=None, directory_id=ROOT_DIR_ID):
        remote_name = remote_name or os.path.basename(local_path)
        host = self.get_upload_host()
        print(f"[3] 上传 {local_path} -> {remote_name}  (host={host})")
        with open(local_path, "rb") as fh:
            content = fh.read()

        batch_no = rand_str(32)
        # fileInfo 必须包含 batchNo 与 spaceType，否则服务端返回「请求参数错误」
        file_info = {"fileName": remote_name, "fileSize": len(content), "fileType": file_type_of(remote_name),
                     "directoryId": directory_id, "batchNo": batch_no, "spaceType": self.space_type}
        fields = {
            "uniqueId": f"{int(time.time() * 1000)}_{rand_str(6)}",
            "accessToken": self.token,
            "fileName": remote_name,
            "psToken": "",
            "fileSize": len(content),
            "totalPart": 1,
            "partSize": len(content),
            "partIndex": 1,
            "channel": "wocloud",
            "directoryId": directory_id,
            "fileInfo": aes_encrypt(json.dumps(file_info, ensure_ascii=False, separators=(",", ":")), self.token),
        }
        resp = self._post_multipart(f"{host}/openapi/client/upload2C", fields, "file", remote_name, content)
        ok = resp.get("code") == "0000"
        print(f"    upload2C -> {'成功' if ok else '失败'} {json.dumps(resp, ensure_ascii=False)[:200]}")
        return ok

    # -- 4. 下载 ----------------------------------------------------------
    def get_download_url(self, fid):
        resp = self.dispatch("GetDownloadUrl", {"fidList": [fid], "clientId": CLIENT_ID,
                                                "spaceType": self.space_type})
        if not self._ok(resp):
            print(f"    [!] 获取下载地址失败：{self._desc(resp)}")
            return None
        data = resp["RSP"].get("DATA") or []
        if not data:
            return None
        return data[0].get("downloadUrl") if isinstance(data[0], dict) else data[0]

    def download(self, file_id, save_path):
        """注意：离线下发接口要传 fid（不是列表里的 id）"""
        print(f"[4] 下载 id={file_id}")
        target = self.find_file("test.txt") or {}
        fid = target.get("fid")
        if not fid:
            print("    [!] 未找到 fid，跳过")
            return False
        url = self.get_download_url(fid)
        if not url:
            return False
        print(f"    downloadUrl = {url[:100]}...")
        req = urllib.request.Request(url)
        req.add_header("accesstoken", self.token)
        try:
            with urllib.request.urlopen(req, timeout=180, context=_CTX) as r, open(save_path, "wb") as out:
                data = r.read()
                out.write(data)
        except Exception as e:
            print(f"    [!] 下载失败: {e}")
            return False
        print(f"    已保存 {save_path} ({len(data)} 字节)")
        return True

    # -- 5. 重命名 --------------------------------------------------------
    def rename(self, file_id, new_name, file_type="4", row_type=1, is_dir=False):
        print(f"[5] 重命名 id={file_id} -> {new_name}")
        params = {"spaceType": self.space_type, "type": 1 if is_dir else row_type,
                  "fileType": file_type, "id": file_id, "name": new_name}
        resp = self.dispatch("RenameFileOrDirectory", params)
        print(f"    RenameFileOrDirectory -> {self._desc(resp)}")
        return self._ok(resp)

    # -- 6. 删除 ----------------------------------------------------------
    def delete(self, file_id, dir_id=None, vip_level="0"):
        print(f"[6] 删除 id={file_id}")
        params = {"spaceType": self.space_type, "vipLevel": vip_level,
                  "dirList": [dir_id] if dir_id else [], "fileList": [file_id]}
        resp = self.dispatch("DeleteFile", params)
        print(f"    DeleteFile -> {self._desc(resp)}")
        return self._ok(resp)


def file_type_of(name: str) -> str:
    """网页版 queryFileType 的分类：0 全部 / 1 图片 / 2 视频 / 3 音频 / 4 文档 / 5 其他"""
    video = {"avi", "asf", "m4v", "dat", "3gp", "m3u8", "m3us", "dv", "flv", "mkv", "webm", "mov",
             "ogv", "mp4", "rm", "swf", "ts", "vob", "wmv", "rmvb", "mpg"}
    audio = {"au", "ac3", "flac", "m4a", "mp2", "mp3", "wav", "wma", "ape", "mpc", "tta", "ogg",
             "amr", "aac", "aiff", "mka", "wv"}
    image = {"jpg", "png", "bmp", "jpeg", "tif", "tiff", "psd", "tga", "raw", "pcd", "heic", "ico",
             "livp", "webp", "gif"}
    doc = {"txt", "doc", "docx", "ppt", "pptx", "xls", "xlsx", "pdf", "rtf", "hlp", "md", "text"}
    ext = name.split(".")[-1].lower() if "." in name else ""
    if ext in image:
        return "1"
    if ext in video:
        return "2"
    if ext in audio:
        return "3"
    if ext in doc:
        return "4"
    return "5"


# ----------------------------------------------------------------------------
def main():
    client = WoCloud()
    print("=" * 60)
    print("联通云盘文件操作脚本")
    print("=" * 60)

    if not client.login():
        print("\n登录未成功，后续步骤中止。")
        return 1

    client.list_files()

    if not os.path.exists(TEST_FILE):
        with open(TEST_FILE, "w", encoding="utf-8") as fh:
            fh.write("hello wocloud\n")
        print(f"[3] 已生成本地测试文件 {TEST_FILE}")

    if not client.upload(TEST_FILE, "test.txt"):
        print("\n[!] 上传失败，中止")
        return 2
    time.sleep(1)

    target = client.find_file("test.txt")
    if not target:
        print("\n[!] 上传后列表里没有 test.txt，中止")
        return 3
    file_id = target.get("id")

    client.download(file_id, DOWNLOAD_TO)

    if client.rename(file_id, "test2.txt", file_type=str(target.get("fileType", "4")),
                     row_type=target.get("type", 1)):
        print("    已重命名 test.txt -> test2.txt")

    # 第 5 步已把 test.txt 改名为 test2.txt，这里删除的是重命名后的文件
    renamed = client.find_file("test2.txt")
    if renamed:
        client.delete(renamed.get("id"))
    else:
        client.delete(file_id)

    print("\n完成。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
