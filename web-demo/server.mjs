import http from 'node:http';
import { randomBytes } from 'node:crypto';
import { readFile, mkdtemp, rm } from 'node:fs/promises';
import { createWriteStream } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { Readable, Transform } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import { ApiError, BaiduApi, MAX_UPLOAD_SIZE, validateName, validatePath, baiduLogin, fetchCaptchaImage, getQrCode, fetchQrImage, pollQrStatus, completeQrLogin } from './baidu-api.mjs';

const SESSION_AGE = 8 * 60 * 60 * 1000;
const PUBLIC = new URL('./public/', import.meta.url);
const ASSETS = { '/': ['index.html', 'text/html'], '/app.js': ['app.js', 'text/javascript'], '/style.css': ['style.css', 'text/css'] };
const sessionCookie = (value, age = SESSION_AGE / 1000) => `pan_demo_session=${value}; HttpOnly; SameSite=Strict; Path=/; Max-Age=${age}`;

function sendJson(res, status, data) {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
  res.end(JSON.stringify(data));
}

async function readJson(req) {
  if (!req.headers['content-type']?.startsWith('application/json')) throw new ApiError('请求必须使用 JSON。', 415);
  let size = 0;
  const chunks = [];
  for await (const chunk of req) {
    size += chunk.length;
    if (size > 32768) throw new ApiError('请求体过大。', 413);
    chunks.push(chunk);
  }
  try {
    const data = JSON.parse(Buffer.concat(chunks).toString());
    if (!data || typeof data !== 'object' || Array.isArray(data)) throw new Error();
    return data;
  } catch { throw new ApiError('JSON 格式不正确。'); }
}

export function createApp({ fetchImpl = fetch } = {}) {
  const sessions = new Map();
  const qrCodes = new Map();
  const cleanup = setInterval(() => {
    const now = Date.now();
    for (const [key, session] of sessions) if (session.expires <= now) sessions.delete(key);
    for (const [key, qr] of qrCodes) if (qr.expires <= now) qrCodes.delete(key);
  }, 60000).unref();

  const server = http.createServer(async (req, res) => {
    res.setHeader('Cache-Control', 'no-store');
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('X-Frame-Options', 'DENY');
    res.setHeader('Referrer-Policy', 'no-referrer');
    res.setHeader('Content-Security-Policy', "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'");
    let sessionId;
    try {
      const host = req.headers.host;
      if (!host || !/^(127\.0\.0\.1|localhost)(:\d+)?$/.test(host)) throw new ApiError('仅允许本机访问。', 403);
      const origin = `http://${host}`;
      const url = new URL(req.url, origin);
      if (req.headers.origin && req.headers.origin !== origin) throw new ApiError('已拒绝跨站请求。', 403);
      if (req.headers['sec-fetch-site'] === 'cross-site') throw new ApiError('已拒绝跨站请求。', 403);
      if (!['GET', 'HEAD'].includes(req.method) && req.headers.origin !== origin) throw new ApiError('请求缺少同源校验。', 403);

      if (ASSETS[url.pathname] && ['GET', 'HEAD'].includes(req.method)) {
        const [name, type] = ASSETS[url.pathname];
        const content = await readFile(new URL(name, PUBLIC));
        res.writeHead(200, { 'Content-Type': `${type}; charset=utf-8` });
        res.end(req.method === 'HEAD' ? undefined : content);
        return;
      }
      if (url.pathname === '/favicon.ico') { res.writeHead(204); res.end(); return; }
      if (!url.pathname.startsWith('/api/')) throw new ApiError('页面不存在。', 404);
      sessionId = req.headers.cookie?.match(/(?:^|;\s*)pan_demo_session=([a-f0-9]{64})(?:;|$)/)?.[1];
      let session = sessions.get(sessionId);
      if (session && session.expires <= Date.now()) { sessions.delete(sessionId); session = undefined; }

      if (url.pathname === '/api/session' && req.method === 'GET') {
        sendJson(res, 200, { authenticated: Boolean(session), maxUploadSize: MAX_UPLOAD_SIZE });
        return;
      }
      if (url.pathname === '/api/login' && req.method === 'POST') {
        const body = await readJson(req);
        if (!body.username || !body.password) throw new ApiError('请输入用户名和密码。');
        const result = await baiduLogin(body.username, body.password);
        const api = new BaiduApi(result, fetchImpl);
        await api.list('/', 0, 1);
        const id = randomBytes(32).toString('hex');
        if (sessionId) sessions.delete(sessionId);
        sessions.set(id, { api, expires: Date.now() + SESSION_AGE, downloads: new Map(), uploading: false });
        res.setHeader('Set-Cookie', sessionCookie(id));
        sendJson(res, 200, { authenticated: true });
        return;
      }
      if (url.pathname === '/api/captcha' && req.method === 'GET') {
        const sig = url.searchParams.get('sig');
        if (!sig) throw new ApiError('缺少验证码参数。');
        const { buffer, contentType } = await fetchCaptchaImage(sig);
        res.writeHead(200, { 'Content-Type': contentType, 'Cache-Control': 'no-store' });
        res.end(buffer);
        return;
      }
      if (url.pathname === '/api/qr/create' && req.method === 'POST') {
        const qr = await getQrCode();
        qrCodes.set(qr.sign, { imageUrl: qr.imageUrl, expires: Date.now() + 300000 });
        sendJson(res, 200, { sign: qr.sign });
        return;
      }
      if (url.pathname === '/api/qr/image' && req.method === 'GET') {
        const sign = url.searchParams.get('sign');
        if (!sign) throw new ApiError('缺少二维码参数。');
        const qr = qrCodes.get(sign);
        if (!qr || qr.expires <= Date.now()) throw new ApiError('二维码已过期，请刷新。', 410);
        const { buffer, contentType } = await fetchQrImage(qr.imageUrl);
        res.writeHead(200, { 'Content-Type': contentType, 'Cache-Control': 'no-store' });
        res.end(buffer);
        return;
      }
      if (url.pathname === '/api/qr/poll' && req.method === 'GET') {
        const sign = url.searchParams.get('sign');
        if (!sign) throw new ApiError('缺少二维码参数。');
        const result = await pollQrStatus(sign);
        if (result.status === 'scanned') {
          try {
            const credentials = await completeQrLogin(sign);
            const api = new BaiduApi(credentials, fetchImpl);
            await api.list('/', 0, 1);
            const id = randomBytes(32).toString('hex');
            if (sessionId) sessions.delete(sessionId);
            sessions.set(id, { api, expires: Date.now() + SESSION_AGE, downloads: new Map(), uploading: false });
            res.setHeader('Set-Cookie', sessionCookie(id));
            sendJson(res, 200, { status: 'success', authenticated: true });
            return;
          } catch (error) {
            if (error instanceof ApiError) { sendJson(res, error.status, { error: error.message }); return; }
            throw error;
          }
        }
        sendJson(res, 200, { status: result.status });
        return;
      }
      if (url.pathname === '/api/logout' && req.method === 'POST') {
        sessions.delete(sessionId);
        res.setHeader('Set-Cookie', sessionCookie('', 0));
        sendJson(res, 200, { ok: true });
        return;
      }
      if (!session) throw new ApiError('请先登录百度网盘。', 401);
      const api = session.api;

      if (url.pathname === '/api/files' && req.method === 'GET') {
        const dir = validatePath(url.searchParams.get('dir') ?? '/');
        const start = Number(url.searchParams.get('start') ?? 0);
        if (!Number.isSafeInteger(start) || start < 0) throw new ApiError('分页参数无效。');
        sendJson(res, 200, await api.list(dir, start));
      } else if (url.pathname === '/api/quota' && req.method === 'GET') {
        const { total, used } = await api.quota();
        sendJson(res, 200, { total, used });
      } else if (url.pathname === '/api/folders' && req.method === 'POST') {
        const { dir, name } = await readJson(req);
        const path = `${validatePath(dir) === '/' ? '' : dir}/${validateName(name)}`;
        await api.mkdir(validatePath(path, false));
        sendJson(res, 200, { ok: true });
      } else if (url.pathname === '/api/rename' && req.method === 'POST') {
        const { path, name } = await readJson(req);
        sendJson(res, 200, await api.manage('rename', [{ path: validatePath(path, false), newname: validateName(name) }]));
      } else if (url.pathname === '/api/delete' && req.method === 'POST') {
        const { paths } = await readJson(req);
        if (!Array.isArray(paths) || !paths.length || paths.length > 100) throw new ApiError('请选择 1–100 个文件。');
        sendJson(res, 200, await api.manage('delete', paths.map((path) => validatePath(path, false))));
      } else if (url.pathname === '/api/upload' && req.method === 'POST') {
        const dir = validatePath(url.searchParams.get('dir') ?? '/');
        const name = validateName(url.searchParams.get('name'));
        const path = validatePath(`${dir === '/' ? '' : dir}/${name}`, false);
        if (session.uploading) throw new ApiError('已有文件正在上传，请等待完成。', 409);
        if (Number(req.headers['content-length']) > MAX_UPLOAD_SIZE) throw new ApiError('Demo 单文件上传上限为 100 MiB。', 413);
        session.uploading = true;
        let temp;
        try {
          temp = await mkdtemp(join(tmpdir(), 'baidu-web-upload-'));
          const localPath = join(temp, 'upload');
          let size = 0;
          const limiter = new Transform({ transform(chunk, encoding, callback) {
            size += chunk.length;
            callback(size > MAX_UPLOAD_SIZE ? new ApiError('Demo 单文件上传上限为 100 MiB。', 413) : null, chunk);
          } });
          await pipeline(req, limiter, createWriteStream(localPath, { mode: 0o600 }));
          await api.upload(localPath, path, size);
          sendJson(res, 200, { ok: true });
        } finally {
          session.uploading = false;
          if (temp) await rm(temp, { recursive: true, force: true });
        }
      } else if (url.pathname === '/api/download' && req.method === 'POST') {
        const { path } = await readJson(req);
        validatePath(path, false);
        const target = await api.locateDownload(path);
        for (const [key, ticket] of session.downloads) if (ticket.expires <= Date.now()) session.downloads.delete(key);
        const id = randomBytes(24).toString('hex');
        session.downloads.set(id, { target, name: path.split('/').pop(), expires: Date.now() + 60000 });
        sendJson(res, 200, { url: `/api/download/${id}` });
      } else if (/^\/api\/download\/[a-f0-9]{48}$/.test(url.pathname) && req.method === 'GET') {
        const id = url.pathname.split('/').pop();
        const ticket = session.downloads.get(id);
        session.downloads.delete(id);
        if (!ticket || ticket.expires <= Date.now()) throw new ApiError('下载链接已过期，请重新点击下载。', 410);
        const controller = new AbortController();
        res.on('close', () => controller.abort());
        const response = await api.request(ticket.target, { signal: AbortSignal.any([controller.signal, AbortSignal.timeout(30 * 60000)]) });
        if (response.headers.get('content-type')?.includes('application/json')) {
          await response.body?.cancel();
          throw new ApiError('百度未返回文件内容，请检查账号下载权限。', 502);
        }
        const filename = encodeURIComponent(ticket.name).replace(/['()*]/g, (char) => `%${char.charCodeAt(0).toString(16)}`);
        res.setHeader('Content-Type', 'application/octet-stream');
        res.setHeader('Content-Disposition', `attachment; filename="download"; filename*=UTF-8''${filename}`);
        const length = response.headers.get('content-length');
        if (length) res.setHeader('Content-Length', length);
        await pipeline(Readable.fromWeb(response.body), res);
      } else {
        throw new ApiError('接口不存在或请求方式不正确。', 404);
      }
    } catch (error) {
      if (error.status === 401 && sessionId) {
        sessions.delete(sessionId);
        res.setHeader('Set-Cookie', sessionCookie('', 0));
      }
      if (res.headersSent || res.destroyed) { res.destroy(); return; }
      const status = error instanceof ApiError ? error.status : 502;
      const message = error instanceof ApiError ? error.message : '请求未完成，请检查网络连接后重试。';
      sendJson(res, status, { error: message, ...(error.code === undefined ? {} : { code: error.code }) });
    }
  });
  server.requestTimeout = 10 * 60 * 1000;
  server.on('close', () => { clearInterval(cleanup); sessions.clear(); qrCodes.clear(); });
  return server;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const port = Number(process.env.PORT || 3000);
  const server = createApp();
  server.on('error', (error) => { console.error(`启动失败：${error.message}`); process.exitCode = 1; });
  server.listen(port, '127.0.0.1', () => console.log(`百度网盘 Web Demo: http://127.0.0.1:${port}\n凭证仅保存在内存；请勿将此本地 Demo 暴露到公网。`));
}
