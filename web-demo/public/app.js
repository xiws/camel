const $ = (selector) => document.querySelector(selector);
const state = { authenticated: false, dir: '/', files: [], selected: new Set(), hasMore: false, loading: false, busy: false, dialog: null, generation: 0 };
let toastTimer;
let qrPollTimer = null;
let qrSign = null;
let activeTab = 'qr';

function stopQrPolling() {
  if (qrPollTimer) { clearInterval(qrPollTimer); qrPollTimer = null; }
  qrSign = null;
}

async function startQrLogin() {
  stopQrPolling();
  $('#qr-loading').hidden = false;
  $('#qr-loading').textContent = '正在生成二维码…';
  $('#qr-image').hidden = true;
  $('#qr-refresh').hidden = true;
  $('#qr-status').textContent = '';
  try {
    const result = await request('/api/qr/create', {});
    qrSign = result.sign;
    $('#qr-loading').hidden = true;
    $('#qr-image').src = `/api/qr/image?sign=${encodeURIComponent(qrSign)}`;
    $('#qr-image').hidden = false;
    qrPollTimer = setInterval(pollQrCode, 2000);
  } catch (error) {
    $('#qr-loading').textContent = error.message;
    $('#qr-refresh').hidden = false;
  }
}

async function pollQrCode() {
  if (!qrSign) return;
  try {
    const result = await request(`/api/qr/poll?sign=${encodeURIComponent(qrSign)}`);
    if (result.status === 'success') {
      stopQrPolling();
      await enterDrive();
    } else if (result.status === 'scanned') {
      $('#qr-status').textContent = '已扫码，请在手机上确认登录…';
    } else if (result.status === 'error') {
      stopQrPolling();
      $('#qr-status').textContent = '二维码已失效，请点击刷新。';
      $('#qr-refresh').hidden = false;
    }
  } catch (error) {
    stopQrPolling();
    $('#qr-status').textContent = error.message;
    $('#qr-refresh').hidden = false;
  }
}

function switchLoginTab(tab) {
  activeTab = tab;
  for (const btn of document.querySelectorAll('.login-tab')) btn.classList.toggle('active', btn.dataset.tab === tab);
  const isQr = tab === 'qr';
  $('#qr-login-section').hidden = !isQr;
  $('#login-form').hidden = isQr;
  if (isQr) startQrLogin(); else stopQrPolling();
}

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function toast(message, error = false) {
  clearTimeout(toastTimer);
  $('#toast').textContent = message;
  $('#toast').className = `toast${error ? ' error' : ''}`;
  $('#toast').hidden = false;
  toastTimer = setTimeout(() => { $('#toast').hidden = true; }, error ? 6500 : 3500);
}

function showLogin() {
  state.authenticated = false;
  state.generation++;
  state.files = [];
  state.selected.clear();
  $('#file-list').replaceChildren();
  $('#search-input').value = '';
  $('#quota-text').textContent = '正在读取空间信息…';
  $('#quota-progress').value = 0;
  $('#drive-view').hidden = true;
  $('#login-view').hidden = false;
  $('#action-dialog').close();
  captchaState = null;
  $('#captcha-section').hidden = true;
  $('#login-form').reset();
  switchLoginTab('qr');
}

async function request(path, body) {
  const response = await fetch(path, body === undefined ? {} : {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  });
  const data = await response.json();
  if (!response.ok) {
    if (response.status === 401 && state.authenticated) showLogin();
    throw new Error(data.error || `请求失败（${response.status}）`);
  }
  return data;
}

function formatSize(bytes) {
  if (!Number.isFinite(Number(bytes))) return '—';
  if (Number(bytes) === 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  const index = Math.min(Math.floor(Math.log(Number(bytes)) / Math.log(1024)), units.length - 1);
  return `${(Number(bytes) / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`;
}

function formatDate(value) {
  if (!value) return '—';
  const date = new Date(Number(value) * 1000);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
}

function fileIcon(file) {
  if (Number(file.isdir)) return element('span', 'file-icon folder');
  const extension = file.server_filename.split('.').pop().toLowerCase();
  let type = 'document';
  if (extension === 'pdf') type = 'pdf';
  else if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'heic'].includes(extension)) type = 'image';
  else if (['mp4', 'mov', 'mkv', 'avi'].includes(extension)) type = 'video';
  return element('span', `file-icon ${type}`, extension.slice(0, 4).toUpperCase());
}

function visibleFiles() {
  const query = $('#search-input').value.trim().toLocaleLowerCase();
  return state.files.filter((file) => file.server_filename.toLocaleLowerCase().includes(query));
}

function selectedFiles() {
  return state.files.filter((file) => state.selected.has(file.path));
}

function updateControls() {
  const locked = state.loading || state.busy;
  for (const id of ['upload-button', 'mkdir-button', 'refresh-button', 'home-button', 'load-more', 'logout-button']) $(`#${id}`).disabled = locked;
  $('#search-input').disabled = locked;
  $('#file-panel').setAttribute('aria-busy', String(locked));
  for (const node of document.querySelectorAll('#file-list button, #file-list input, #breadcrumbs button')) node.disabled = locked;
  const selected = selectedFiles();
  $('#selection-bar').hidden = !selected.length;
  $('#selection-count').textContent = `已选择 ${selected.length} 项`;
  $('#rename-button').disabled = locked || selected.length !== 1;
  $('#download-button').disabled = locked || selected.length !== 1 || Boolean(Number(selected[0]?.isdir));
  $('#delete-button').disabled = locked || !selected.length;
  $('#clear-selection').disabled = locked;
  const visible = visibleFiles();
  $('#select-all').disabled = locked || !visible.length;
  $('#select-all').checked = visible.length > 0 && visible.every((file) => state.selected.has(file.path));
  $('#select-all').indeterminate = !$('#select-all').checked && visible.some((file) => state.selected.has(file.path));
}

function renderBreadcrumbs() {
  const root = $('#breadcrumbs');
  root.replaceChildren();
  const parts = state.dir.split('/').filter(Boolean);
  const crumbs = [{ name: '全部文件', path: '/' }, ...parts.map((name, index) => ({ name, path: `/${parts.slice(0, index + 1).join('/')}` }))];
  for (const [index, crumb] of crumbs.entries()) {
    if (index) root.append(element('span', '', '/'));
    const button = element('button', '', crumb.name);
    button.title = crumb.path;
    button.onclick = () => loadFiles(crumb.path);
    root.append(button);
  }
}

function renderFiles() {
  const visible = visibleFiles();
  const rows = [];
  for (const file of visible) {
    const row = element('tr', state.selected.has(file.path) ? 'selected' : '');
    const checkCell = element('td', 'checkbox-cell');
    const checkbox = element('input');
    checkbox.type = 'checkbox';
    checkbox.checked = state.selected.has(file.path);
    checkbox.setAttribute('aria-label', `选择 ${file.server_filename}`);
    checkbox.onchange = () => {
      if (checkbox.checked) state.selected.add(file.path); else state.selected.delete(file.path);
      renderFiles();
    };
    checkCell.append(checkbox);
    const nameCell = element('td');
    const name = element('div', 'file-name');
    const nameButton = element('button', '', file.server_filename);
    nameButton.title = `${Number(file.isdir) ? '打开文件夹' : '下载文件'}：${file.server_filename}`;
    nameButton.onclick = () => Number(file.isdir) ? loadFiles(file.path) : download(file);
    name.append(fileIcon(file), nameButton);
    nameCell.append(name);
    const actions = element('td', 'action-cell');
    const rename = element('button', 'text-button', '重命名');
    rename.setAttribute('aria-label', `重命名 ${file.server_filename}`);
    rename.onclick = () => openDialog('rename', [file]);
    const remove = element('button', 'text-button danger-text', '删除');
    remove.setAttribute('aria-label', `删除 ${file.server_filename}`);
    remove.onclick = () => openDialog('delete', [file]);
    actions.append(rename, remove);
    row.append(checkCell, nameCell, element('td', 'size-cell', Number(file.isdir) ? '—' : formatSize(file.size)), element('td', 'date-cell', formatDate(file.server_mtime || file.local_mtime)), actions);
    rows.push(row);
  }
  $('#file-list').replaceChildren(...rows);
  $('#file-count').textContent = $('#search-input').value ? `${visible.length} / ${state.files.length} 项` : `已加载 ${state.files.length} 项`;
  $('#load-more').hidden = !state.hasMore;
  const empty = $('#list-state');
  empty.className = 'list-state';
  empty.hidden = visible.length > 0;
  empty.replaceChildren(element('strong', '', $('#search-input').value ? '没有匹配的文件' : '文件夹还是空的'), element('span', '', $('#search-input').value ? '试试其他关键词，或加载更多文件。' : '上传你的第一个文件，或新建一个文件夹。'));
  updateControls();
}

async function loadFiles(dir = state.dir, append = false) {
  if (state.loading || state.busy || !state.authenticated) return;
  state.loading = true;
  const generation = ++state.generation;
  if (!append) {
    state.dir = dir;
    state.files = [];
    state.selected.clear();
    state.hasMore = false;
    $('#search-input').value = '';
    $('#file-list').replaceChildren();
    $('#list-state').hidden = false;
    $('#list-state').className = 'list-state';
    $('#list-state').textContent = '正在加载文件…';
    $('#file-count').textContent = '';
    $('#load-more').hidden = true;
  }
  renderBreadcrumbs();
  updateControls();
  try {
    const data = await request(`/api/files?${new URLSearchParams({ dir, start: append ? state.files.length : 0 })}`);
    if (generation !== state.generation) return;
    state.files = append ? [...state.files, ...data.list] : data.list;
    state.hasMore = data.hasMore;
    renderFiles();
  } catch (error) {
    if (generation !== state.generation) return;
    if (!append) {
      $('#list-state').textContent = error.message;
      $('#list-state').className = 'list-state error';
      $('#list-state').hidden = false;
    }
    toast(error.message, true);
  } finally {
    state.loading = false;
    updateControls();
  }
}

async function loadQuota() {
  try {
    const quota = await request('/api/quota');
    if (!state.authenticated) return;
    $('#quota-text').textContent = `${formatSize(quota.used)} / ${formatSize(quota.total)}`;
    $('#quota-progress').value = quota.total > 0 ? Math.min(100, quota.used / quota.total * 100) : 0;
  } catch (error) {
    $('#quota-text').textContent = '空间信息暂不可用';
    $('#quota-text').title = error.message;
  }
}

async function enterDrive() {
  state.authenticated = true;
  state.loading = false;
  state.busy = false;
  $('#login-view').hidden = true;
  $('#drive-view').hidden = false;
  await Promise.all([loadFiles('/'), loadQuota()]);
}

function openDialog(kind, files = []) {
  if (state.busy || state.loading) return;
  state.dialog = { kind, files, dir: state.dir };
  $('#dialog-error').hidden = true;
  $('#dialog-title').textContent = { mkdir: '新建文件夹', rename: '重命名', delete: '确认删除' }[kind];
  $('#dialog-description').textContent = kind === 'delete'
    ? `将从网盘删除 ${files.length === 1 ? `「${files[0].server_filename}」` : `选中的 ${files.length} 个项目`}。删除文件夹也会删除其中的内容，请确认后继续。`
    : kind === 'mkdir' ? `创建位置：${state.dir}` : `当前名称：${files[0].server_filename}`;
  $('#name-field').hidden = kind === 'delete';
  $('#name-input').required = kind !== 'delete';
  $('#name-input').value = kind === 'rename' ? files[0].server_filename : '';
  $('#dialog-confirm').textContent = kind === 'delete' ? '确认删除' : '确认';
  $('#dialog-confirm').className = `button ${kind === 'delete' ? 'danger-button' : 'primary'}`;
  $('#action-dialog').showModal();
  if (kind === 'delete') $('#dialog-cancel').focus(); else { $('#name-input').focus(); $('#name-input').select(); }
}

async function download(file) {
  if (!file || state.busy || state.loading) return;
  state.busy = true;
  updateControls();
  try {
    const data = await request('/api/download', { path: file.path });
    const link = element('a');
    link.href = data.url;
    link.download = file.server_filename;
    document.body.append(link);
    link.click();
    link.remove();
    toast('下载已交给浏览器，请查看浏览器下载列表。');
  } catch (error) { toast(error.message, true); }
  finally { state.busy = false; updateControls(); }
}

function uploadOne(file, dir) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', `/api/upload?${new URLSearchParams({ dir, name: file.name })}`);
    xhr.setRequestHeader('Content-Type', 'application/octet-stream');
    xhr.timeout = 30 * 60 * 1000;
    xhr.upload.onprogress = (event) => {
      if (!event.lengthComputable) return;
      $('#transfer-progress').value = event.loaded / event.total * 100;
      $('#transfer-detail').textContent = `发送到本地服务：${Math.round(event.loaded / event.total * 100)}%`;
    };
    xhr.upload.onload = () => {
      $('#transfer-progress').removeAttribute('value');
      $('#transfer-detail').textContent = '正在计算 MD5 并分片上传到百度网盘，请稍候…';
    };
    xhr.onload = () => {
      if (xhr.status === 401) showLogin();
      let data;
      try { data = JSON.parse(xhr.responseText); } catch { reject(new Error('上传响应异常。')); return; }
      if (xhr.status >= 200 && xhr.status < 300) resolve(data); else reject(new Error(data.error || '上传失败。'));
    };
    xhr.onerror = () => reject(new Error('上传连接中断，请检查网络后重试。'));
    xhr.ontimeout = () => reject(new Error('上传超时，请刷新网盘检查文件是否已经保存。'));
    xhr.send(file);
  });
}

async function uploadFiles(files) {
  if (!files.length || state.busy || state.loading || !state.authenticated) return;
  if (files.some((file) => file.size > 100 * 1024 * 1024)) { toast('Demo 单文件上传上限为 100 MiB，请重新选择。', true); return; }
  state.busy = true;
  const dir = state.dir;
  updateControls();
  $('#transfer').hidden = false;
  let completed = 0;
  try {
    for (const file of files) {
      $('#transfer-title').textContent = `上传 ${completed + 1}/${files.length} · ${file.name}`;
      $('#transfer-detail').textContent = '正在发送到本地服务…';
      $('#transfer-progress').value = 0;
      await uploadOne(file, dir);
      completed++;
    }
    toast(`已上传 ${completed} 个文件。`);
  } catch (error) { toast(`${error.message}${completed ? ` 已完成 ${completed} 个文件。` : ''}`, true); }
  finally {
    state.busy = false;
    $('#transfer').hidden = true;
    $('#file-input').value = '';
    updateControls();
    if (state.authenticated) await Promise.all([loadFiles(dir), loadQuota()]);
  }
}

let captchaState = null;

for (const tab of document.querySelectorAll('.login-tab')) tab.onclick = () => switchLoginTab(tab.dataset.tab);
$('#qr-refresh').onclick = () => startQrLogin();

$('#login-form').onsubmit = async (event) => {
  event.preventDefault();
  const button = $('#login-submit');
  button.disabled = true;
  button.textContent = '正在登录…';
  $('#login-error').hidden = true;
  try {
    const body = { username: $('#username').value.trim(), password: $('#password').value };
    if (captchaState) {
      const vcode = $('#captcha-input').value.trim();
      if (!vcode) throw new Error('请输入验证码。');
      body.captcha = { as: captchaState.captchaAs, ds: captchaState.captchaDs, tk: captchaState.captchaTk, vcode };
    }
    const result = await request('/api/login', body);
    if (result.needCaptcha) {
      captchaState = { captchaAs: result.captchaAs, captchaDs: result.captchaDs, captchaTk: result.captchaTk };
      $('#captcha-section').hidden = false;
      $('#captcha-image').src = `/api/captcha?sig=${encodeURIComponent(result.vcodeSig)}`;
      $('#captcha-input').value = '';
      $('#captcha-input').focus();
      button.disabled = false;
      button.textContent = '登录百度网盘 →';
      return;
    }
    $('#login-form').reset();
    captchaState = null;
    $('#captcha-section').hidden = true;
    await enterDrive();
  } catch (error) {
    $('#login-error').textContent = error.message;
    $('#login-error').hidden = false;
  } finally { button.disabled = false; button.textContent = '登录百度网盘 →'; }
};

$('#logout-button').onclick = async () => {
  try { await request('/api/logout', {}); showLogin(); toast('已退出，本地会话已清除。'); }
  catch (error) { toast(error.message, true); }
};
$('#home-button').onclick = () => loadFiles('/');
$('#refresh-button').onclick = () => loadFiles();
$('#load-more').onclick = () => loadFiles(state.dir, true);
$('#search-input').oninput = () => { state.selected.clear(); renderFiles(); };
$('#select-all').onchange = (event) => {
  for (const file of visibleFiles()) if (event.target.checked) state.selected.add(file.path); else state.selected.delete(file.path);
  renderFiles();
};
$('#clear-selection').onclick = () => { state.selected.clear(); renderFiles(); };
$('#mkdir-button').onclick = () => openDialog('mkdir');
$('#rename-button').onclick = () => openDialog('rename', selectedFiles());
$('#delete-button').onclick = () => openDialog('delete', selectedFiles());
$('#download-button').onclick = () => download(selectedFiles()[0]);
$('#upload-button').onclick = () => $('#file-input').click();
$('#file-input').onchange = () => uploadFiles([...$('#file-input').files]);
for (const id of ['dialog-close', 'dialog-cancel']) $(`#${id}`).onclick = () => { if (!state.busy) $('#action-dialog').close(); };
$('#action-dialog').addEventListener('cancel', (event) => { if (state.busy) event.preventDefault(); });
$('#action-form').onsubmit = async (event) => {
  event.preventDefault();
  if (state.busy) return;
  state.busy = true;
  updateControls();
  for (const node of $('#action-form').querySelectorAll('button, input')) node.disabled = true;
  $('#dialog-error').hidden = true;
  const { kind, files, dir } = state.dialog;
  let success = false;
  try {
    let result;
    if (kind === 'mkdir') result = await request('/api/folders', { dir, name: $('#name-input').value });
    if (kind === 'rename') result = await request('/api/rename', { path: files[0].path, name: $('#name-input').value });
    if (kind === 'delete') result = await request('/api/delete', { paths: files.map((file) => file.path) });
    $('#action-dialog').close();
    toast(result.pending ? '操作已提交，百度正在异步处理，请稍后刷新。' : { mkdir: '文件夹已创建。', rename: '重命名成功。', delete: '所选文件已删除。' }[kind]);
    success = true;
  } catch (error) {
    $('#dialog-error').textContent = error.message;
    $('#dialog-error').hidden = false;
    if (!state.authenticated) toast(error.message, true);
  } finally {
    state.busy = false;
    for (const node of $('#action-form').querySelectorAll('button, input')) node.disabled = false;
    updateControls();
    if (success) await Promise.all([loadFiles(dir), loadQuota()]);
  }
};

let dragDepth = 0;
$('#file-panel').addEventListener('dragenter', (event) => { event.preventDefault(); dragDepth++; if (!state.busy) $('#file-panel').classList.add('dragging'); });
$('#file-panel').addEventListener('dragover', (event) => event.preventDefault());
$('#file-panel').addEventListener('dragleave', () => { if (--dragDepth <= 0) $('#file-panel').classList.remove('dragging'); });
$('#file-panel').addEventListener('drop', (event) => {
  event.preventDefault();
  dragDepth = 0;
  $('#file-panel').classList.remove('dragging');
  uploadFiles([...event.dataTransfer.files]);
});

request('/api/session').then((session) => session.authenticated ? enterDrive() : showLogin()).catch((error) => { showLogin(); toast(error.message, true); });
