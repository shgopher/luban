// 自包含探针：拉起 headless Chrome -> CDP 测量 -> 关闭。无第三方依赖（Node 26 内置）。
// 页面需在内容进入 DOM 时调用 performance.mark('content-ready')，用于算「内容就绪时间」。
const { spawn } = require('node:child_process');
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');

const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const PORT = Number(process.env.T73_PORT || 9400 + Math.floor(Math.random()*200));
const HTTP_PORT = Number(process.env.T73_HTTP || 18000 + Math.floor(Math.random()*500));
const sleep = ms => new Promise(r => setTimeout(r, ms));

function startServer(root) {
  const types = { '.html': 'text/html; charset=utf-8', '.js': 'application/javascript', '.json': 'application/json', '.css': 'text/css' };
  const srv = http.createServer((req, res) => {
    const p = path.join(root, decodeURIComponent(req.url.split('?')[0]));
    fs.readFile(p, (err, buf) => {
      if (err) { res.writeHead(404); return res.end('not found'); }
      res.writeHead(200, { 'Content-Type': types[path.extname(p)] || 'application/octet-stream', 'Cache-Control': 'no-store' });
      res.end(buf);
    });
  });
  return new Promise(r => srv.listen(HTTP_PORT, '127.0.0.1', () => r(srv)));
}

async function launch() {
  const dir = fs.mkdtempSync('/tmp/t73-chrome-');
  const proc = spawn(CHROME, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage',
    `--remote-debugging-port=${PORT}`, '--no-first-run', '--no-default-browser-check',
    `--user-data-dir=${dir}`, 'about:blank'], { stdio: 'ignore' });
  let up = false;
  for (let i = 0; i < 50; i++) {
    try { const r = await fetch(`http://127.0.0.1:${PORT}/json/version`); if (r.ok) { up = true; break; } } catch {}
    await sleep(200);
  }
  if (!up) throw new Error('chrome 未就绪（检查是否与用户 Chrome 冲突）');
  for (let i = 0; i < 25; i++) {
    try { const l = await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json(); if (l.some(t => t.type === 'page')) break; } catch {}
    await sleep(200);
  }
  return { proc, dir };
}

async function connect() {
  const list = await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json();
  const page = list.find(t => t.type === 'page');
  if (!page) throw new Error('no page target');
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((res, rej) => { ws.addEventListener('open', res, { once: true }); ws.addEventListener('error', rej, { once: true }); });
  let id = 0; const pending = new Map();
  ws.addEventListener('message', ev => {
    const m = JSON.parse(ev.data);
    if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  });
  const send = (method, params = {}) => new Promise((res, rej) => {
    const i = ++id;
    const t = setTimeout(() => { pending.delete(i); rej(new Error(method + ' 超时')); }, 20000);
    pending.set(i, m => { clearTimeout(t); m.error ? rej(new Error(method + ': ' + m.error.message)) : res(m.result); });
    ws.send(JSON.stringify({ id: i, method, params }));
  });
  return { ws, send };
}

async function measureOnce(send, url, { throttle, cpu = 1, waitMs = 2000 }) {
  await send('Network.setCacheDisabled', { cacheDisabled: true });
  if (throttle) {
    await send('Network.emulateNetworkConditions', {
      offline: false, latency: throttle.latency,
      downloadThroughput: Math.round(throttle.mbps * 1024 * 1024 / 8),
      uploadThroughput: Math.round(throttle.mbps * 0.4 * 1024 * 1024 / 8),
      connectionType: 'cellular4g',
    });
  } else {
    await send('Network.emulateNetworkConditions', { offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1, connectionType: 'wifi' });
  }
  await send('Emulation.setCPUThrottlingRate', { rate: cpu });
  await send('Page.navigate', { url });
  await sleep(waitMs);
  const r = await send('Runtime.evaluate', { returnByValue: true, expression: `(() => {
    const nav = performance.getEntriesByType('navigation')[0] || {};
    const res = performance.getEntriesByType('resource');
    const sum = (a,k) => a.reduce((s,x)=>s+(x[k]||0),0);
    const ready = performance.getEntriesByName('content-ready');
    const lt = performance.getEntriesByType('longtask');
    return { ttfb: Math.round(nav.responseStart||0), dcl: Math.round(nav.domContentLoadedEventEnd||0),
      load: Math.round(nav.loadEventEnd||0),
      ready: ready.length ? Math.round(ready[0].startTime) : null,
      longtaskMs: Math.round(sum(lt,'duration')), longtaskN: lt.length,
      res: res.length, jsReq: res.filter(x=>x.name.endsWith('.js')).length,
      wireBytes: sum(res,'transferSize') + (nav.transferSize||0),
      decodedBytes: sum(res,'decodedBodySize') + (nav.decodedBodySize||0) };
  })()` });
  return r.result.value;
}

const median = a => { const s = [...a].sort((x, y) => x - y); return s[Math.floor(s.length / 2)]; };

async function measure(send, url, opts = {}, runs = 5) {
  const rows = [];
  for (let i = 0; i < runs + 1; i++) {           // 第 1 次热身丢弃
    const r = await measureOnce(send, url, opts);
    if (i > 0) rows.push(r);
  }
  const keys = Object.keys(rows[0]);
  const out = {};
  for (const k of keys) out[k] = median(rows.map(r => r[k] ?? 0));
  out.runs = runs;
  return out;
}

module.exports = { startServer, launch, connect, measure, measureOnce, sleep, HTTP_PORT, median };

if (require.main === module) {
  (async () => {
    const root = process.argv[2];
    const urls = process.argv.slice(3);
    const srv = await startServer(root);
    const { proc, dir } = await launch();
    const { ws, send } = await connect();
    await send('Page.enable'); await send('Runtime.enable'); await send('Network.enable');
    await send('Page.addScriptToEvaluateOnNewDocument', { source: `
      new PerformanceObserver(l => { for (const e of l.getEntries()) window.__lcp = e.startTime; }).observe({type:'largest-contentful-paint', buffered:true});
      new PerformanceObserver(l => { for (const e of l.getEntries()) if (!e.hadRecentInput) window.__cls = (window.__cls||0)+e.value; }).observe({type:'layout-shift', buffered:true});
    `});
    const out = {};
    for (const u of urls) {
      out[u + ' fast'] = await measure(send, `http://127.0.0.1:${HTTP_PORT}${u}`, {});
      out[u + ' 4g+cpu4x'] = await measure(send, `http://127.0.0.1:${HTTP_PORT}${u}`, { throttle: { latency: 150, mbps: 1.6 }, cpu: 4 });
    }
    console.log(JSON.stringify(out, null, 1));
    ws.close(); proc.kill('SIGKILL'); srv.close();
    fs.rmSync(dir, { recursive: true, force: true });
  })().catch(e => { console.error('ERR', e.message); process.exit(1); });
}
