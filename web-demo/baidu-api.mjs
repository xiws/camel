import { createHash, publicEncrypt, constants } from 'node:crypto';
import { open } from 'node:fs/promises';

const PAN = 'https://pan.baidu.com/api/';
const PCS = 'https://d.pcs.baidu.com/rest/2.0/pcs/file';
export const BLOCK_SIZE = 4 * 1024 * 1024;
export const MAX_UPLOAD_SIZE = 100 * 1024 * 1024;
export const md5 = (value) => createHash('md5').update(value).digest('hex');

export class ApiError extends Error {
  constructor(message, status = 400, code) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export function validatePath(value, allowRoot = true) {
  if (typeof value !== 'string' || !value.startsWith('/') || value.length > 4096 ||
      /[\x00-\x1f\x7f\\]/.test(value) || value.split('/').some((part) => part === '.' || part === '..') ||
      value.includes('//') || (value.length > 1 && value.endsWith('/')) || (!allowRoot && value === '/')) {
    throw new ApiError('网盘路径不合法。');
  }
  return value;
}

export function validateName(value) {
  if (typeof value !== 'string' || !value.trim() || value !== value.trim() ||
      value.length > 255 || /[\\/:*?"<>|\x00-\x1f\x7f]/.test(value) || value === '.' || value === '..') {
    throw new ApiError('名称不能为空、首尾不能有空格，也不能包含 / \\ : * ? " < > | 等字符。');
  }
  return value;
}

export function parseCredentials(input, stoken = '') {
  if (typeof input !== 'string' || typeof stoken !== 'string' || input.length > 16384 || /[\r\n]/.test(input)) {
    throw new ApiError('请输入有效的 BDUSS 或 Cookie。');
  }
  const allowed = new Set(['BDUSS', 'STOKEN', 'panPSC', 'ndut_fmt', 'ndFTID']);
  const values = new Map();
  const raw = input.trim().replace(/^Cookie:\s*/i, '');
  if (/^(?:BDUSS|STOKEN|[^;=]+)=/.test(raw) && /(?:^|;\s*)BDUSS=/.test(raw)) {
    for (const pair of raw.split(';')) {
      const index = pair.indexOf('=');
      const key = pair.slice(0, index).trim();
      if (index > 0 && allowed.has(key)) values.set(key, pair.slice(index + 1).trim());
    }
  } else {
    values.set('BDUSS', raw);
  }
  if (stoken.trim()) values.set('STOKEN', stoken.trim());
  for (const value of values.values()) {
    if (!value || !/^[\x21-\x3a\x3c-\x7e]+$/.test(value)) throw new ApiError('Cookie 格式不正确，请勿包含空白或换行。');
  }
  if (!values.get('BDUSS')) throw new ApiError('Cookie 中缺少 BDUSS。');
  return {
    cookie: [...values].map(([key, value]) => `${key}=${value}`).join('; '),
    token: md5(values.get('BDUSS')),
  };
}

const PASSPORT_UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36';

function parsePassportCookies(resp) {
  const raw = resp.headers.get('set-cookie') || '';
  if (!raw) return '';
  return raw.split(/,(?=\s*\w+=)/).map((c) => c.trim()).filter(Boolean).map((c) => c.split(';')[0].trim()).join('; ');
}

function parsePassportJson(text) {
  return JSON.parse(text.replace(/'([^']*)'/g, '"$1"'));
}

function extractLoginResult(html) {
  const params = {};
  const hrefMatch = html.match(/href\s*\+=\s*"([^"]+)"/);
  if (hrefMatch) {
    for (const pair of hrefMatch[1].split('&')) {
      const eq = pair.indexOf('=');
      if (eq > 0) params[pair.slice(0, eq)] = decodeURIComponent(pair.slice(eq + 1));
    }
  }
  return params;
}

export async function baiduLogin(username, password) {
  const pageResp = await fetch('https://passport.baidu.com/v2/?login&tpl=netdisk&u=https://pan.baidu.com/', {
    headers: { 'User-Agent': PASSPORT_UA, Accept: 'text/html' },
    redirect: 'manual',
    signal: AbortSignal.timeout(15000),
  });
  const pageText = await pageResp.text();
  const cookieStr = parsePassportCookies(pageResp);

  const allHex = pageText.match(/[a-f0-9]{32}/g);
  const token = allHex?.[0] || '';
  if (!token) throw new ApiError('获取登录令牌失败。', 502);

  const keyResp = await fetch(`https://passport.baidu.com/v2/getpublickey?token=${token}&tpl=netdisk&apiver=v3&tt=${Date.now()}`, {
    headers: { 'User-Agent': PASSPORT_UA, Referer: 'https://passport.baidu.com/v2/?login', Cookie: cookieStr },
    signal: AbortSignal.timeout(10000),
  });
  let keyData;
  try { keyData = parsePassportJson(await keyResp.text()); } catch { throw new ApiError('获取加密公钥失败。', 502); }
  if (!keyData.pubkey) throw new ApiError('百度未返回公钥，请稍后重试。', 502);

  const encPassword = publicEncrypt(
    { key: keyData.pubkey, padding: constants.RSA_PKCS1_PADDING },
    Buffer.from(password),
  ).toString('hex');

  const loginParams = new URLSearchParams({
    staticpage: 'https://pan.baidu.com/res/static/thirdparty/loginResult.html',
    charset: 'UTF-8', token, tpl: 'netdisk', apiver: 'v3',
    tt: String(Date.now()), username, password: encPassword,
    isphone: '0', loginmerge: '1', adapter: '3',
    u: 'https://pan.baidu.com/', papi: '1', sms: '0',
  });

  const loginResp = await fetch('https://passport.baidu.com/v2/api/?login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      'User-Agent': PASSPORT_UA,
      Referer: 'https://passport.baidu.com/v2/?login&tpl=netdisk',
      Cookie: cookieStr,
    },
    body: loginParams,
    redirect: 'manual',
    signal: AbortSignal.timeout(30000),
  });
  const loginText = await loginResp.text();
  const result = extractLoginResult(loginText);
  const errNo = result.err_no;

  if (errNo === '0') {
    const bduss = result.bduss || '';
    const stoken = result.stoken || '';
    if (bduss) return parseCredentials(`BDUSS=${bduss}${stoken ? `; STOKEN=${stoken}` : ''}`);
  }

  if (errNo === '50052' || errNo === '50001') {
    throw new ApiError('百度要求短信或安全验证，请使用扫码登录，或在百度官方客户端完成验证后重试。', 401);
  }

  const errorMessages = {
    '4': '账号或密码错误，请重试。',
    '6': '验证码错误，请重新输入。',
    '257': '需要验证码。',
    '100006': '系统繁忙，请稍后重试。',
  };
  throw new ApiError(errorMessages[errNo] || `登录失败 (${errNo || '未知'})`, 401);
}

export async function fetchCaptchaImage(vcodeSig) {
  const response = await fetch(`https://passport.baidu.com/cgi-bin/genimage?${encodeURIComponent(vcodeSig)}`, {
    headers: { 'User-Agent': PASSPORT_UA, Referer: 'https://passport.baidu.com/' },
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new ApiError('获取验证码图片失败。', 502);
  const buffer = Buffer.from(await response.arrayBuffer());
  const contentType = response.headers.get('content-type') || 'image/jpeg';
  return { buffer, contentType };
}

const QR_APP_ID = '250528';

export async function getQrCode() {
  const response = await fetch(`https://passport.baidu.com/v2/api/getqrcode?client_id=${QR_APP_ID}&tpl=netdisk&apiver=v3&tt=${Date.now()}&lp=pc`, {
    headers: { 'User-Agent': PASSPORT_UA, Referer: 'https://passport.baidu.com/' },
    signal: AbortSignal.timeout(15000),
  });
  let data;
  try { data = await response.json(); } catch { throw new ApiError('获取二维码失败。', 502); }
  if (Number(data.errno) !== 0 || !data.sign) throw new ApiError('获取二维码失败。', 502);
  return { sign: data.sign, imageUrl: `https://${data.imgurl}` };
}

export async function fetchQrImage(imageUrl) {
  const response = await fetch(imageUrl, {
    headers: { 'User-Agent': PASSPORT_UA, Referer: 'https://passport.baidu.com/' },
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new ApiError('获取二维码图片失败。', 502);
  const buffer = Buffer.from(await response.arrayBuffer());
  return { buffer, contentType: response.headers.get('content-type') || 'image/png' };
}

export async function pollQrStatus(sign) {
  const response = await fetch(`https://passport.baidu.com/channel/unicast?sign=${sign}&tpl=netdisk&apiver=v3&tt=${Date.now()}`, {
    headers: { 'User-Agent': PASSPORT_UA, Referer: 'https://passport.baidu.com/' },
    signal: AbortSignal.timeout(30000),
  });
  let data;
  try { data = await response.json(); } catch { return { status: 'error' }; }
  const code = Number(data.errno);
  if (code === 0) return { status: 'scanned', channelId: data.channel_id || '' };
  if (code === -1) return { status: 'waiting' };
  return { status: 'error' };
}

export async function completeQrLogin(sign) {
  const response = await fetch(`https://passport.baidu.com/v3/login/main/qrbdusslogin?v=${Date.now()}&sign=${sign}&client_id=${QR_APP_ID}&tpl=netdisk`, {
    headers: { 'User-Agent': PASSPORT_UA, Referer: 'https://passport.baidu.com/' },
    redirect: 'manual',
    signal: AbortSignal.timeout(15000),
  });
  const text = await response.text();
  let data;
  try { data = JSON.parse(text.replace(/'([^']*)'/g, '"$1"')); } catch { throw new ApiError('扫码登录响应格式异常。', 502); }
  const code = data.code || data.errInfo?.no || '';
  if (String(code) === '0') {
    const bduss = data.data?.session?.bduss || '';
    if (bduss) return parseCredentials(`BDUSS=${bduss}`);
  }
  const msg = data.message || data.errInfo?.msg || '';
  throw new ApiError(msg || `扫码登录失败 (${code})`, 401);
}

export function trustedUrl(value) {
  let url;
  try { url = new URL(value); } catch { throw new ApiError('百度返回了无效的传输地址。', 502); }
  const trusted = ['baidu.com', 'baidupcs.com'].some((domain) => url.hostname === domain || url.hostname.endsWith(`.${domain}`));
  if (url.protocol !== 'https:' || !trusted || url.username || url.password || (url.port && url.port !== '443')) {
    throw new ApiError('传输地址不是受信任的百度 HTTPS 节点，已停止发送凭证。', 502);
  }
  return url;
}

function checkResult(data) {
  if (!data || typeof data !== 'object' || Array.isArray(data)) throw new ApiError('百度接口响应格式异常。', 502);
  const code = Number(data.errno ?? data.error_code ?? 0);
  if (code !== 0) {
    const messages = {
      '-6': '登录凭证已失效，请重新登录。',
      '-7': '文件名不合法或无权操作。',
      '-8': '同名文件或文件夹已存在。',
      '-9': '文件或文件夹不存在，请刷新列表。',
      '-19': '百度要求验证码，请先在官方客户端完成验证。',
      '132': '百度要求安全验证，请先在官方客户端完成验证。',
    };
    throw new ApiError(messages[code] || `百度接口请求失败（${code}）：${String(data.show_msg || data.errmsg || data.error_msg || data.error_info || '未知错误').slice(0, 200)}`, code === -6 ? 401 : 502, code);
  }
  return data;
}

export class BaiduApi {
  constructor(credentials, fetchImpl = fetch) {
    this.credentials = credentials;
    this.fetch = fetchImpl;
  }

  async request(url, options = {}) {
    let current = trustedUrl(url);
    for (let redirects = 0; redirects < 5; redirects++) {
      const response = await this.fetch(current, {
        ...options,
        headers: {
          'User-Agent': 'netdisk;9.15.1.32;Android;web-demo',
          Referer: 'https://pan.baidu.com/',
          ...options.headers,
          Cookie: this.credentials.cookie,
        },
        redirect: 'manual',
        signal: options.signal ?? AbortSignal.timeout(120000),
      });
      if ([301, 302, 303, 307, 308].includes(response.status)) {
        const location = response.headers.get('location');
        await response.body?.cancel();
        if (!location || (options.method && options.method !== 'GET')) throw new ApiError('百度接口发生异常跳转。', 502);
        current = trustedUrl(new URL(location, current).href);
        continue;
      }
      if (!response.ok) {
        await response.body?.cancel();
        throw new ApiError(`百度服务返回 HTTP ${response.status}。`, 502);
      }
      return response;
    }
    throw new ApiError('百度服务重定向次数过多。', 502);
  }

  async json(url, options) {
    const response = await this.request(url, options);
    let data;
    try { data = await response.json(); } catch { throw new ApiError('百度未返回 JSON，可能需要重新登录或完成安全验证。', 502); }
    return checkResult(data);
  }

  pan(endpoint, query = {}, form) {
    const url = new URL(endpoint, PAN);
    for (const [key, value] of Object.entries({ ...query, bdstoken: this.credentials.token, app_id: '250528', clienttype: '1' })) {
      url.searchParams.set(key, value);
    }
    return this.json(url, form ? { method: 'POST', body: new URLSearchParams(form) } : {});
  }

  async list(dir, start = 0, limit = 100) {
    const data = await this.pan('list', { dir, start, limit, order: 'name', desc: 0, preset: 0 });
    if (!Array.isArray(data.list)) throw new ApiError('百度未返回文件列表，请检查登录凭证。', 502);
    return { list: data.list, hasMore: data.has_more === undefined ? data.list.length === limit : Boolean(Number(data.has_more)) };
  }

  quota() {
    return this.pan('quota', { checkrecycle: 1, needquota: 1 });
  }

  mkdir(path) {
    const timestamp = Math.floor(Date.now() / 1000);
    return this.pan('create', { a: 'commit', norename: '' }, {
      path, size: 0, isdir: 1, local_ctime: timestamp, local_mtime: timestamp, block_list: '[]',
    });
  }

  async manage(operation, files) {
    const data = await this.pan('filemanager', { opera: operation, async: 0, ondup: 1 }, { filelist: JSON.stringify(files) });
    const failure = data.info?.find((item) => ![0, ...(operation === 'delete' ? [-9, 31066] : [])].includes(Number(item.errno ?? 0)));
    if (failure) checkResult(failure);
    return { pending: Number(data.taskid ?? data.task_id ?? 0) > 0 };
  }

  async upload(localPath, path, size) {
    const file = await open(localPath, 'r');
    try {
      const blocks = [];
      const contentHash = createHash('md5');
      for (let offset = 0; offset < size || offset === 0; offset += BLOCK_SIZE) {
        const buffer = Buffer.alloc(Math.min(BLOCK_SIZE, size - offset));
        const { bytesRead } = await file.read(buffer, 0, buffer.length, offset);
        if (bytesRead !== buffer.length) throw new ApiError('本地上传文件读取不完整。', 500);
        contentHash.update(buffer);
        blocks.push(md5(buffer));
      }
      const form = { path, size, isdir: 0, block_list: JSON.stringify(blocks), rtype: 0 };
      const precreate = await this.pan('precreate', {}, { ...form, autoinit: 1, 'content-md5': contentHash.digest('hex') });
      if (Number(precreate.return_type) === 2) return { rapid: true };
      if (!precreate.uploadid || !Array.isArray(precreate.block_list)) throw new ApiError('预创建响应缺少上传 ID 或分片列表。', 502);
      const indexes = precreate.block_list.map(Number);
      if (indexes.some((index) => !Number.isInteger(index) || index < 0 || index >= blocks.length)) throw new ApiError('百度返回了无效的分片索引。', 502);
      if (indexes.length) {
        const locateUrl = new URL(PCS);
        locateUrl.search = new URLSearchParams({ method: 'locateupload', upload_version: '2.0', app_id: '250528', ...(precreate.uploadsign ? { uploadsign: precreate.uploadsign } : {}) });
        const located = await this.json(locateUrl);
        const server = located.servers?.[0];
        if (!server?.server) throw new ApiError('百度未返回可用的上传节点。', 502);
        const base = trustedUrl(server.server);
        for (const index of indexes) {
          const offset = index * BLOCK_SIZE;
          const buffer = Buffer.alloc(Math.min(BLOCK_SIZE, size - offset));
          await file.read(buffer, 0, buffer.length, offset);
          const url = new URL('/rest/2.0/pcs/superfile2', base);
          url.search = new URLSearchParams({ method: 'upload', type: 'tmpfile', path, partoffset: offset, app_id: '250528', uploadid: precreate.uploadid, partseq: index });
          const body = new FormData();
          body.append('file', new Blob([buffer]), 'chunk');
          const result = await this.json(url, { method: 'POST', body });
          if (result.md5 !== blocks[index]) throw new ApiError('上传分片 MD5 校验失败，请重试。', 502);
        }
      }
      return await this.pan('create', {}, { ...form, uploadid: precreate.uploadid });
    } finally {
      await file.close();
    }
  }

  async locateDownload(path) {
    const url = new URL(PCS);
    url.search = new URLSearchParams({ method: 'locatedownload', path, ver: '2.0', dtype: 0, esl: 1, ehps: 1, app_id: '250528', check_blue: 1 });
    const data = await this.json(url);
    const location = data.urls?.[0]?.url;
    if (!location) throw new ApiError('百度未返回下载地址，文件可能受下载限制。', 502);
    return trustedUrl(location).href;
  }
}
