import { md5 } from '../baidu-api.mjs';

export const TEST_CREDENTIAL = 'demo-test-session';

export function createFixtureFetch() {
  const now = Math.floor(Date.now() / 1000);
  let id = 100;
  const files = new Map();
  const uploads = new Map();
  const calls = [];
  const json = (value) => Response.json(value);
  function add(path, isdir = 0, content = '') {
    const data = Buffer.from(content);
    files.set(path, { fs_id: ++id, path, server_filename: path.split('/').pop(), isdir, size: isdir ? 0 : data.length, server_mtime: now, content: data });
  }
  add('/工作文档', 1);
  add('/旅行照片', 1);
  add('/项目归档', 1);
  add('/产品设计说明.pdf', 0, 'PDF fixture');
  add('/周末旅行.jpg', 0, 'Image fixture');
  add('/会议记录.txt', 0, '这是一份用于本地测试的会议记录。\n');
  add('/工作文档/项目计划.txt', 0, 'Project plan');

  const fetchImpl = async (input, options = {}) => {
    const url = new URL(input);
    calls.push({ url: url.href, options });
    if (!options.headers.Cookie?.split('; ').includes(`BDUSS=${TEST_CREDENTIAL}`)) return json({ errno: -6 });
    if (url.pathname.startsWith('/api/') && url.searchParams.get('bdstoken') !== md5(TEST_CREDENTIAL)) return json({ errno: -6 });
    const body = options.body instanceof URLSearchParams ? Object.fromEntries(options.body) : {};
    if (url.pathname === '/api/list') {
      const dir = url.searchParams.get('dir');
      if (dir !== '/' && !files.get(dir)?.isdir) return json({ errno: -9 });
      const list = [...files.values()].filter((file) => file.path.slice(0, file.path.lastIndexOf('/')) === (dir === '/' ? '' : dir))
        .sort((a, b) => b.isdir - a.isdir || a.server_filename.localeCompare(b.server_filename, 'zh-CN'));
      const start = Number(url.searchParams.get('start'));
      const limit = Number(url.searchParams.get('limit'));
      return json({ errno: 0, list: list.slice(start, start + limit).map(({ content, ...file }) => file), has_more: Number(start + limit < list.length) });
    }
    if (url.pathname === '/api/quota') return json({ errno: 0, total: 2 * 1024 ** 4, used: 36.8 * 1024 ** 3 });
    if (url.pathname === '/api/create') {
      if (files.has(body.path)) return json({ errno: -8 });
      const content = body.isdir === '1' ? '' : Buffer.concat(uploads.get(body.uploadid)?.parts || []);
      add(body.path, Number(body.isdir), content);
      return json({ errno: 0, path: body.path, fs_id: id });
    }
    if (url.pathname === '/api/filemanager') {
      const entries = JSON.parse(body.filelist);
      const operation = url.searchParams.get('opera');
      const info = [];
      for (const entry of entries) {
        const path = typeof entry === 'string' ? entry : entry.path;
        if (!files.has(path)) { info.push({ path, errno: -9 }); continue; }
        const target = operation === 'rename' ? `${path.slice(0, path.lastIndexOf('/'))}/${entry.newname}` : null;
        if (target && files.has(target)) { info.push({ path, errno: -8 }); continue; }
        for (const [key, file] of [...files]) {
          if (key === path || key.startsWith(`${path}/`)) {
            files.delete(key);
            if (target) {
              const newPath = target + key.slice(path.length);
              files.set(newPath, { ...file, path: newPath, server_filename: newPath.split('/').pop() });
            }
          }
        }
        info.push({ path, errno: 0 });
      }
      return json({ errno: 0, info });
    }
    if (url.pathname === '/api/precreate') {
      const uploadid = `upload-${uploads.size + 1}`;
      const hashes = JSON.parse(body.block_list);
      uploads.set(uploadid, { parts: [], hashes, body });
      return json({ errno: 0, uploadid, uploadsign: 'fixture-sign', block_list: hashes.map((_, index) => index), return_type: 1 });
    }
    if (url.searchParams.get('method') === 'locateupload') return json({ servers: [{ server: 'https://d.pcs.baidu.com' }] });
    if (url.pathname.endsWith('/superfile2')) {
      const data = Buffer.from(await options.body.get('file').arrayBuffer());
      const upload = uploads.get(url.searchParams.get('uploadid'));
      upload.parts[Number(url.searchParams.get('partseq'))] = data;
      return json({ md5: md5(data) });
    }
    if (url.searchParams.get('method') === 'locatedownload') {
      const path = url.searchParams.get('path');
      if (!files.has(path)) return json({ error_code: 31066 });
      return json({ urls: [{ url: `https://d.pcs.baidu.com/fixture-download?path=${encodeURIComponent(path)}` }] });
    }
    if (url.pathname === '/fixture-download') {
      const file = files.get(url.searchParams.get('path'));
      return new Response(file.content, { headers: { 'Content-Type': 'application/octet-stream', 'Content-Length': String(file.content.length) } });
    }
    throw new Error(`Unhandled fixture: ${url.pathname}`);
  };
  return { fetchImpl, files, uploads, calls, add };
}
