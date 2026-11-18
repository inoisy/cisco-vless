'use strict';

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const EXIT_NAMES = { vless: 'VLESS', wg: 'WireGuard', awg: 'AmneziaWG' };

async function api(method, path, body) {
  const r = await fetch('/api' + path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(data.error || r.statusText);
  return data;
}

function flash(text, isError) {
  const el = $('#flash');
  el.textContent = text;
  el.className = 'flash' + (isError ? ' error' : '');
  el.hidden = !text;
  clearTimeout(flash.t);
  if (text && !isError) flash.t = setTimeout(() => (el.hidden = true), 5000);
}

// busy: кнопка неактивна, пока идёт запрос; результат — во flash.
async function busy(btn, fn) {
  btn.classList.add('busy');
  try {
    const res = await fn();
    if (res && res.message) flash(res.message);
    return res;
  } catch (e) {
    flash(e.message, true);
  } finally {
    btn.classList.remove('busy');
    refresh();
  }
}

// --- вкладки ---------------------------------------------------------------

function showTab(name) {
  const tab = name === 'connect' ? 'status' : name;
  $$('.tabs button').forEach(b => b.classList.toggle('active', b.dataset.tab === tab));
  $$('.tab').forEach(t => t.classList.toggle('active', t.id === 'tab-' + tab));
  if (tab === 'settings') loadSettings();
  if (tab === 'logs') loadLogs();
  if (name === 'connect') setTimeout(() => $('#otp').focus(), 50);
}
window.cvpnShow = showTab; // зовёт трей
// Трей: cisco отвалилась сама — просим новый код.
window.cvpnDropped = () => {
  showTab('connect');
  flash('Cisco отключилась (сервер выкинул сессию) — введи новый код и нажми Enter', true);
};
$$('.tabs button').forEach(b => b.addEventListener('click', () => showTab(b.dataset.tab)));
document.addEventListener('click', e => {
  const g = e.target.closest('[data-goto]');
  if (g) { e.preventDefault(); showTab(g.dataset.goto); }
});

// --- статус ----------------------------------------------------------------

let status = null;

async function refresh(withIP) {
  try {
    status = await api('GET', '/status' + (withIP ? '?ip=1' : ''));
    $('#daemon-down').hidden = true;
  } catch (e) {
    status = null;
    $('#daemon-down').hidden = false;
  }
  render(withIP);
}

function render(withIP) {
  const s = status;
  const down = !s;
  $$('#tab-status button').forEach(b => (b.disabled = down));

  const pill = (el, on, text) => { el.textContent = text; el.classList.toggle('on', on); };
  pill($('#cisco-pill'), !!(s && s.cisco), s ? (s.cisco ? 'подключён' : 'выключен') : '—');
  pill($('#exit-pill'), !!(s && s.exit), s ? (s.exit ? EXIT_NAMES[s.exit] : 'напрямую') : '—');

  $('#cisco-off-ctl').hidden = !s || s.cisco || !s.configured;
  $('#cisco-on-ctl').hidden = !s || !s.cisco;
  $('#cisco-unconf').hidden = !s || s.configured;
  if (s && s.cisco && s.corp_host) {
    $('#corp').textContent = `${s.corp_host} → ${s.corp_ip || 'НЕ РЕЗОЛВИТСЯ'}`;
  } else {
    $('#corp').textContent = '';
  }

  $$('#exits button').forEach(b => {
    const e = b.dataset.exit;
    b.classList.toggle('active', !!s && s.exit === e);
    b.disabled = down || (e !== '' && !s.exits.includes(e));
    b.title = b.disabled && !down ? 'не настроен — см. Настройки' : '';
  });

  $('#all-on').disabled = down || !s.configured;
  $('#all-off').disabled = down || (!s.cisco && !s.exit);
  if (withIP && s) $('#ip').textContent = s.ip || 'нет ответа';

  $('#warnings').innerHTML = '';
  (s && s.warnings || []).forEach(w => {
    const li = document.createElement('li');
    li.textContent = w;
    $('#warnings').append(li);
  });
}

$('#cisco-on').addEventListener('click', e =>
  busy(e.target, () => api('POST', '/cisco/on', { otp: $('#otp').value.trim() })).then(() => ($('#otp').value = '')));
$('#otp').addEventListener('keydown', e => { if (e.key === 'Enter') $('#cisco-on').click(); });
$('#cisco-off').addEventListener('click', e => busy(e.target, () => api('POST', '/cisco/off')));
$$('#exits button').forEach(b => b.addEventListener('click', () => {
  const e = b.dataset.exit;
  busy(b, () => api('POST', e ? `/exit/${e}/on` : '/exit/off')).then(() => refresh(true));
}));
$('#all-on').addEventListener('click', e => {
  const exit = status && status.exits.includes('vless') ? 'vless' : (status && status.exits[0]) || '';
  busy(e.target, () => api('POST', '/on', { exit, otp: $('#otp').value.trim() }))
    .then(() => { $('#otp').value = ''; refresh(true); });
});
$('#all-off').addEventListener('click', e => busy(e.target, () => api('POST', '/off')).then(() => refresh(true)));
$('#ip-refresh').addEventListener('click', () => { $('#ip').textContent = '…'; refresh(true); });

// --- настройки -------------------------------------------------------------

const form = $('#settings');
let settings = null;

async function loadSettings() {
  try {
    settings = await api('GET', '/settings');
  } catch (e) {
    flash(e.message, true);
    return;
  }
  for (const el of form.elements) {
    if (!el.name || el.type === 'file' || el.type === 'checkbox') continue;
    const v = settings[el.name];
    el.value = Array.isArray(v) ? v.join(' ') : (v ?? '');
  }
  form.vless_link.value = (settings.vless_link || '').split(/\s+/).filter(Boolean).join('\n');
  form.wg_conf.value = form.awg_conf.value = '';
  form.wg_del.checked = form.awg_del.checked = false;
  $('#wg-state').textContent = settings.has_wg ? '— загружен' : '— нет';
  $('#awg-state').textContent = settings.has_awg ? '— загружен' : '— нет';
  $('#probe-confirm').hidden = true;
  loadPasswordState();
}

async function loadPasswordState() {
  try {
    const r = await api('GET', '/password');
    $('#password-state').textContent = r.has ? 'сохранён' : 'не сохранён';
  } catch {
    $('#password-state').textContent = '';
  }
}

form.addEventListener('submit', async e => {
  e.preventDefault();
  const f = form;
  const body = {
    cisco_host: f.cisco_host.value.trim(),
    cisco_user: f.cisco_user.value.trim(),
    cisco_group: f.cisco_group.value.trim(),
    cisco_pin: f.cisco_pin.value.trim(),
    corp_domains: f.corp_domains.value.split(/[\s,]+/).filter(Boolean),
    corp_check_host: f.corp_check_host.value.trim(),
    vless_link: f.vless_link.value.split(/\s+/).filter(Boolean).join(' '),
    boot_exit: f.boot_exit.value,
    openconnect: f.openconnect.value.trim(),
    vpnc_script: f.vpnc_script.value.trim(),
    sing_box: f.sing_box.value.trim(),
  };
  for (const k of ['wg', 'awg']) {
    if (f[k + '_del'].checked) body[k + '_conf'] = '';
    else if (f[k + '_conf'].value.trim()) body[k + '_conf'] = f[k + '_conf'].value;
  }
  if (body.cisco_host && !body.cisco_pin) {
    flash('Получи и подтверди сертификат сервера Cisco', true);
    return;
  }
  await busy(e.submitter || $('button[type=submit]', form), async () => {
    const r = await api('PUT', '/settings', body);
    await loadSettings();
    const warn = (r && r.warnings) || [];
    return { message: ['Настройки сохранены', ...warn].join('\n') };
  });
});

$('#probe').addEventListener('click', e => busy(e.target, async () => {
  const host = form.cisco_host.value.trim();
  if (!host) throw new Error('Сначала укажи сервер');
  const r = await api('POST', '/probe', { host });
  if (r.pin === form.cisco_pin.value) return { message: 'Сертификат не изменился' };
  $('#probe-pin').textContent = r.pin;
  $('#probe-ip').textContent = r.ip;
  $('#probe-confirm').hidden = false;
}));
$('#probe-ok').addEventListener('click', () => {
  form.cisco_pin.value = $('#probe-pin').textContent;
  $('#probe-confirm').hidden = true;
  flash('Сертификат принят — не забудь «Сохранить»');
});
$('#probe-no').addEventListener('click', () => ($('#probe-confirm').hidden = true));

$('#password-save').addEventListener('click', e => {
  const pw = $('#password').value;
  if (!pw) return flash('Пустой пароль', true);
  if (!settings || !settings.cisco_user) return flash('Сначала сохрани логин', true);
  busy(e.target, () => api('POST', '/password', { password: pw }))
    .then(() => { $('#password').value = ''; loadPasswordState(); });
});

$$('.reveal').forEach(b => b.addEventListener('click', () => {
  const inp = form[b.dataset.for];
  let hidden;
  if (inp.tagName === 'TEXTAREA') hidden = inp.classList.toggle('secret');
  else { inp.type = inp.type === 'password' ? 'text' : 'password'; hidden = inp.type === 'password'; }
  b.textContent = hidden ? 'показать' : 'скрыть';
}));

$$('input[type=file][data-into]').forEach(inp => inp.addEventListener('change', async () => {
  const file = inp.files[0];
  if (file) form[inp.dataset.into].value = await file.text();
  inp.value = '';
}));

// --- логи ------------------------------------------------------------------

async function loadLogs() {
  try {
    const l = await api('GET', '/logs');
    $('#log-cisco').textContent = l.cisco || '—';
    $('#log-vless').textContent = l.vless || '—';
    $('#log-daemon').textContent = l.daemon || '—';
    $$('#tab-logs pre').forEach(p => (p.scrollTop = p.scrollHeight));
  } catch (e) {
    flash(e.message, true);
  }
}
$('#logs-refresh').addEventListener('click', loadLogs);

// --- старт -----------------------------------------------------------------

refresh(true);
setInterval(() => { if (!document.hidden) refresh(); }, 3000);
document.addEventListener('visibilitychange', () => { if (!document.hidden) refresh(true); });
