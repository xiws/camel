import test from 'node:test';
import assert from 'node:assert/strict';
import { request as httpRequest } from 'node:http';
import { createApp } from '../server.mjs';
import { BaiduApi, BLOCK_SIZE, MAX_UPLOAD_SIZE, md5, parseCredentials, trustedUrl, validateName, validatePath } from '../baidu-api.mjs';
import { createFixtureFetch, TEST_CREDENTIAL } from './fixtures.mjs';

async function setup(t, override) {
  const fixture = createFixtureFetch();
  const server = createApp({ fetchImpl: override ? (url, options) => override(url, options, fixture.fetchImpl) : fixture.fetchImpl });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  t.after(() => new Promise((resolve) => { server.close(resolve); server.closeAllConnections(); }));
  let cookie = '';
  async function request(path, body, options = {}) {
    const response = await fetch(`${base}${path}`, {
      ...(body === undefined ? {} : { method: 'POST', body: JSON.stringify(body) }),
      ...options,
      headers: { Origin: base, Cookie: cookie, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...options.headers },
    });
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) cookie = setCookie.split(';')[0];
    return response;
  }
  async function login() {
    const response = await request('/api/login', { credential: TEST_CREDENTIAL });
    assert.equal(response.status, 200);
    return response;
  }
  return { ...fixture, request, login, base };
}

test('Cookie parsing only keeps supported credentials and derives lowercase MD5', () => {
  assert.deepEqual(parseCredentials('BDUSS=test-token; STOKEN=extra; ignored=secret'), { cookie: 'BDUSS=test-token; STOKEN=extra', token: md5('test-token') });
  assert.equal(parseCredentials('test-token', 'short').cookie, 'BDUSS=test-token; STOKEN=short');
  assert.throws(() => parseCredentials('test\r\nInjected: header'));
  assert.throws(() => parseCredentials(''));
  assert.throws(() => parseCredentials('token', 'x; BDUSS=other'));
});

test('Paths, root deletion and invalid names are rejected', () => {
  for (const name of ['', ' ', '.', '..', 'a/b', 'a\\b', ' x', '<x>', 'a\u0000']) assert.throws(() => validateName(name));
  for (const path of ['relative', '/a/../b', '/a//b', '/a\n', '/a/']) assert.throws(() => validatePath(path));
  assert.throws(() => validatePath('/', false));
  assert.equal(validateName('中文 文件.txt'), '中文 文件.txt');
  assert.equal(validatePath('/工作/文件.txt'), '/工作/文件.txt');
});

test('Only trusted HTTPS transfer destinations are allowed', () => {
  for (const url of ['http://d.pcs.baidu.com/file', 'https://baidu.com.evil.test/', 'https://127.0.0.1/', 'https://user@d.pcs.baidu.com/', 'https://d.pcs.baidu.com:8443/']) assert.throws(() => trustedUrl(url));
  assert.equal(trustedUrl('https://d.pcs.baidu.com/file').hostname, 'd.pcs.baidu.com');
});

test('External redirect is blocked before forwarding credentials', async () => {
  let calls = 0;
  const api = new BaiduApi(parseCredentials('test-token'), async () => {
    calls++;
    return new Response(null, { status: 302, headers: { location: 'https://evil.test/file' } });
  });
  await assert.rejects(api.request('https://d.pcs.baidu.com/file'), /受信任/);
  assert.equal(calls, 1);
});

test('Static assets work and unauthenticated file access is blocked', async (t) => {
  const app = await setup(t);
  const page = await app.request('/');
  assert.equal(page.status, 200);
  assert.match(page.headers.get('content-security-policy'), /frame-ancestors 'none'/);
  assert.match(await page.text(), /BDUSS \/ Cookie/);
  assert.equal((await app.request('/api/files')).status, 401);
  assert.equal((await app.request('/server.mjs')).status, 404);
});

test('Login verifies credentials, creates HttpOnly session and logout clears it', async (t) => {
  const app = await setup(t);
  assert.equal((await app.request('/api/login', { credential: 'invalid' })).status, 401);
  const login = await app.login();
  assert.match(login.headers.get('set-cookie'), /HttpOnly; SameSite=Strict/);
  assert.doesNotMatch(await login.text(), new RegExp(TEST_CREDENTIAL));
  assert.equal((await (await app.request('/api/session')).json()).authenticated, true);
  const list = await (await app.request('/api/files?dir=%2F')).json();
  assert.equal(list.list.length, 6);
  assert.equal(list.hasMore, false);
  await app.request('/api/logout', {});
  assert.equal((await app.request('/api/files')).status, 401);
});

test('Cross-origin writes and DNS rebinding hosts are rejected', async (t) => {
  const app = await setup(t);
  assert.equal((await app.request('/api/login', { credential: TEST_CREDENTIAL }, { headers: { Origin: 'https://evil.test' } })).status, 403);
  assert.equal((await app.request('/api/login', { credential: TEST_CREDENTIAL }, { headers: { Origin: '' } })).status, 403);
  const hostStatus = await new Promise((resolve, reject) => {
    const req = httpRequest(`${app.base}/api/session`, { headers: { Host: 'evil.test' } }, (res) => { res.resume(); resolve(res.statusCode); });
    req.on('error', reject);
    req.end();
  });
  assert.equal(hostStatus, 403);
  assert.equal(app.calls.length, 0);
});

test('Directory listing supports nested folders and offset pagination', async (t) => {
  const app = await setup(t);
  for (let i = 0; i < 110; i++) app.add(`/分页-${i}.txt`);
  await app.login();
  const page = await (await app.request('/api/files?dir=%2F')).json();
  assert.equal(page.list.length, 100);
  assert.equal(page.hasMore, true);
  const last = await (await app.request('/api/files?dir=%2F&start=100')).json();
  assert.equal(last.list.length, 16);
  assert.equal(last.hasMore, false);
  const child = await (await app.request(`/api/files?dir=${encodeURIComponent('/工作文档')}`)).json();
  assert.equal(child.list[0].server_filename, '项目计划.txt');
  assert.equal((await app.request('/api/files?start=-1')).status, 400);
});

test('Create, rename and delete use documented form fields and preserve failures', async (t) => {
  const app = await setup(t);
  await app.login();
  assert.equal((await app.request('/api/folders', { dir: '/', name: '测试目录' })).status, 200);
  const createCall = app.calls.find(({ url }) => new URL(url).pathname === '/api/create');
  assert.equal(createCall.options.body.get('isdir'), '1');
  assert.equal(createCall.options.body.get('block_list'), '[]');
  assert.equal((await app.request('/api/folders', { dir: '/', name: '测试目录' })).status, 502);
  assert.equal((await app.request('/api/rename', { path: '/测试目录', name: '新名字' })).status, 200);
  assert.ok(app.files.has('/新名字'));
  assert.equal((await app.request('/api/rename', { path: '/新名字', name: '工作文档' })).status, 502);
  assert.equal((await app.request('/api/delete', { paths: ['/新名字'] })).status, 200);
  assert.equal(app.files.has('/新名字'), false);
  const manageCalls = app.calls.filter(({ url }) => new URL(url).pathname === '/api/filemanager');
  assert.ok(manageCalls.every(({ url }) => new URL(url).searchParams.get('async') === '0'));
  assert.deepEqual(JSON.parse(manageCalls.at(-1).options.body.get('filelist')), ['/新名字']);
});

test('Invalid mutation input never reaches the upstream API', async (t) => {
  const app = await setup(t);
  await app.login();
  const count = app.calls.length;
  assert.equal((await app.request('/api/delete', { paths: ['/'] })).status, 400);
  assert.equal((await app.request('/api/delete', { paths: [] })).status, 400);
  assert.equal((await app.request('/api/folders', { dir: '/', name: '../x' })).status, 400);
  assert.equal((await app.request('/api/rename', { path: '/会议记录.txt', name: '' })).status, 400);
  assert.equal((await app.request('/api/login', undefined, { method: 'POST', body: '{bad', headers: { 'Content-Type': 'application/json' } })).status, 400);
  assert.equal(app.calls.length, count);
});

test('Uploads hash and transfer multiple 4 MiB blocks, then finalize the file', async (t) => {
  const app = await setup(t);
  await app.login();
  const content = Buffer.alloc(BLOCK_SIZE + 17, 'a');
  const response = await app.request('/api/upload?dir=%2F&name=upload.bin', undefined, { method: 'POST', body: content, headers: { 'Content-Type': 'application/octet-stream' } });
  assert.equal(response.status, 200);
  assert.deepEqual(app.files.get('/upload.bin').content, content);
  const upload = [...app.uploads.values()][0];
  assert.deepEqual(upload.hashes, [md5(content.subarray(0, BLOCK_SIZE)), md5(content.subarray(BLOCK_SIZE))]);
  assert.equal(upload.body['content-md5'], md5(content));
  const parts = app.calls.filter(({ url }) => new URL(url).pathname.endsWith('/superfile2'));
  assert.equal(parts.length, 2);
  assert.equal(new URL(parts[1].url).searchParams.get('partoffset'), String(BLOCK_SIZE));
  assert.equal(new URL(parts[1].url).searchParams.get('partseq'), '1');
});

test('Empty files can be uploaded', async (t) => {
  const app = await setup(t);
  await app.login();
  const response = await app.request('/api/upload?name=empty.txt', undefined, { method: 'POST', body: Buffer.alloc(0) });
  assert.equal(response.status, 200);
  assert.equal(app.files.get('/empty.txt').size, 0);
});

test('Uploads reject oversized content-length before reading a file', async (t) => {
  const app = await setup(t);
  const login = await app.login();
  const cookie = login.headers.get('set-cookie').split(';')[0];
  const status = await new Promise((resolve, reject) => {
    const req = httpRequest(`${app.base}/api/upload?name=large.bin`, {
      method: 'POST', headers: { Origin: app.base, Cookie: cookie, 'Content-Length': MAX_UPLOAD_SIZE + 1 },
    }, (res) => { res.resume(); resolve(res.statusCode); });
    req.on('error', reject);
    req.end();
  });
  assert.equal(status, 413);
  const response = await app.request('/api/upload?name=bad%2Fname', undefined, { method: 'POST', body: 'a' });
  assert.equal(response.status, 400);
});

test('Rapid upload skips chunk transfer', async (t) => {
  const app = await setup(t, (url, options, fallback) => new URL(url).pathname === '/api/precreate' ? Response.json({ errno: 0, return_type: 2 }) : fallback(url, options));
  await app.login();
  const response = await app.request('/api/upload?name=rapid.txt', undefined, { method: 'POST', body: 'existing content' });
  assert.equal(response.status, 200);
  assert.equal(app.uploads.size, 0);
});

test('Bad chunk checksum stops upload before final create', async (t) => {
  const app = await setup(t, (url, options, fallback) => new URL(url).pathname.endsWith('/superfile2') ? Response.json({ md5: 'wrong' }) : fallback(url, options));
  await app.login();
  const response = await app.request('/api/upload?name=bad.txt', undefined, { method: 'POST', body: 'content' });
  assert.equal(response.status, 502);
  assert.match((await response.json()).error, /MD5/);
  assert.equal(app.files.has('/bad.txt'), false);
});

test('Downloads stream original bytes with safe filename and single-use session ticket', async (t) => {
  const app = await setup(t);
  await app.login();
  const response = await app.request('/api/download', { path: '/会议记录.txt' });
  const { url } = await response.json();
  assert.match(url, /^\/api\/download\/[a-f0-9]{48}$/);
  const download = await app.request(url);
  assert.equal(download.status, 200);
  assert.match(download.headers.get('content-disposition'), /filename\*=UTF-8''/);
  assert.equal(await download.text(), app.files.get('/会议记录.txt').content.toString());
  assert.equal((await app.request(url)).status, 410);
});

test('Safety verification errors are visible and expired credentials clear the session', async (t) => {
  let expired = false;
  const app = await setup(t, (url, options, fallback) => {
    if (expired) return Response.json({ errno: -6 });
    if (new URL(url).pathname === '/api/filemanager') return Response.json({ errno: 132 });
    return fallback(url, options);
  });
  await app.login();
  const deletion = await app.request('/api/delete', { paths: ['/会议记录.txt'] });
  assert.equal(deletion.status, 502);
  assert.match((await deletion.json()).error, /安全验证/);
  expired = true;
  assert.equal((await app.request('/api/files')).status, 401);
  assert.equal((await (await app.request('/api/session')).json()).authenticated, false);
});

test('Async task submission is not reported as completed', async (t) => {
  const app = await setup(t, (url, options, fallback) => new URL(url).pathname === '/api/filemanager' ? Response.json({ errno: 0, taskid: 123 }) : fallback(url, options));
  await app.login();
  const response = await app.request('/api/rename', { path: '/会议记录.txt', name: '新.txt' });
  assert.equal((await response.json()).pending, true);
});

test('Non-JSON upstream responses report errors without leaking credentials', async (t) => {
  const app = await setup(t, () => new Response('<html>login required</html>'));
  const response = await app.request('/api/login', { credential: TEST_CREDENTIAL });
  assert.equal(response.status, 502);
  const text = await response.text();
  assert.match(text, /JSON/);
  assert.doesNotMatch(text, /demo-test-session/);
});
