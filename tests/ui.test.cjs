// Integration smoke test for the actual browser entrypoint (no external packages).
const fs = require('fs');
const vm = require('vm');
const assert = require('assert');
class Node {
  constructor() { this.children = []; this.hidden = false; this.disabled = false; this.value = ''; this.checked = false; this.textContent = ''; this.dataset = {}; this.attrs = {}; this.style={setProperty(){}};this.classList = { toggle() {}, remove() {}, add() {},contains(){return false;} }; }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = nodes; }
  setAttribute(key, value) { this.attrs[key] = value; }
  focus() {}
}
async function boot(feeds, articles, addFeed) {
  const nodes = new Map();
  const markup = fs.readFileSync(require('path').join(__dirname, '../ui/index.html'), 'utf8');
  for (const match of markup.matchAll(/id="([^"]+)"/g)) nodes.set(match[1], new Node());
  const document = { getElementById(id) { if (!nodes.has(id)) nodes.set(id, new Node()); return nodes.get(id); }, createElement() { return new Node(); }, body: new Node(), documentElement: new Node(), addEventListener() {} };
  const calls = [];
  const requests = [];
  const bridge = { ready: Promise.resolve(), theme: { appearance: 'light' }, async invoke(method, params) {
    calls.push(method); requests.push({ method, params });
    if (method === 'rss/addFeed') return addFeed(params);
    if (method === 'rss/getSettings') return { displayMode:'rich', showImages:false, imageWidth:800, density:'comfortable', timeoutSeconds:30, maxArticles:0, browser:'default' };
    if (method === 'rss/listGroups') return [{id:'group',name:'技术'}];
    if (method === 'rss/listBrowsers') return [{id:'default',name:'系统默认浏览器'}];
    if (method === 'rss/fetchImage') return {dataURL:'data:image/png;base64,iVBORw0KGgo='};
    if (method === 'rss/library/index') return {items:[],categories:[]};
    return method === 'rss/listFeeds' ? (typeof feeds === 'function' ? feeds() : feeds) : articles;
  } };
  const context = vm.createContext({ document, window: { dbxPlugin: bridge, addEventListener() {} }, navigator: {}, console, setTimeout, clearTimeout, URL });
  for(const name of ['reading','library'])vm.runInContext(fs.readFileSync(require('path').join(__dirname, '../ui/'+name+'.js'), 'utf8'), context);
  vm.runInContext(fs.readFileSync(require('path').join(__dirname, '../ui/app.js'), 'utf8'), context);
  await new Promise(resolve => setImmediate(resolve));
  return { nodes, calls, requests, context };
}
(async () => {
  for (const [feeds, articles] of [[null, null], [[], { items: null, total: 0 }], [[], { items: [], total: 0 }]]) {
    const { nodes, calls } = await boot(feeds, articles);
    assert.strictEqual(nodes.get('status').textContent, '欢迎，添加你的第一个订阅源');
    assert.strictEqual(nodes.get('feedCount').textContent, '0 个订阅源');
    assert.strictEqual(nodes.get('empty').hidden, false);
    assert(calls.includes('rss/getSettings'), 'load persisted settings');
    assert(calls.includes('rss/listGroups'), 'load groups');
    assert(calls.includes('rss/listArticles'));
    await nodes.get('refreshAll').onclick();
    assert.strictEqual(nodes.get('refreshAll').disabled, false);
    assert.strictEqual(nodes.get('status').textContent, '先添加一个订阅源，再获取最新内容');
  }
  const ui = await boot([], { items: [], total: 0 }, async () => { throw new Error('该订阅已存在'); });
  const resolveImage=attrs=>{ui.context.imageNode={getAttribute(key){return attrs[key]||null;}};return vm.runInContext("imageSource(imageNode,'https://example.com/posts/one')",ui.context);};
  assert.strictEqual(resolveImage({'src':'data:image/gif;base64,AA','data-src':'/real.jpg'}),'https://example.com/real.jpg');
  assert.strictEqual(resolveImage({'src':'javascript:bad','data-original':'//cdn.example.com/real.jpg'}),'https://cdn.example.com/real.jpg');
  assert.strictEqual(resolveImage({'srcset':'/small.jpg 400w, /large.jpg 1200w'}),'https://example.com/large.jpg');
  assert.strictEqual(resolveImage({}), '', 'missing image src must not resolve to the article URL');
  assert.strictEqual(resolveImage({'src':'javascript:alert(1)'}),'');
  ui.context.testImages=Array.from({length:8},()=>({getAttribute(){return 'https://example.com/photo.png';},classList:{add(){},remove(){}}}));
  ui.context.testBody={isConnected:true,querySelectorAll(){return ui.context.testImages;}};
  ui.context.IntersectionObserver=class{constructor(callback){this.callback=callback;}observe(){}unobserve(){}disconnect(){}};
  vm.runInContext('state.settings.showImages=true; hydrateImages(testBody)',ui.context);
  await new Promise(resolve=>setImmediate(resolve));
  assert.strictEqual(ui.requests.filter(r=>r.method==='rss/fetchImage').length,6,'first images should load concurrently even when an observer has not fired');
  assert(ui.context.testImages.slice(0,6).every(i=>i.src?.startsWith('data:image/png;')),'proxy results must be assigned to visible images');
  assert.strictEqual(typeof ui.nodes.get('submitAdd').onclick, 'function', 'sandbox must submit through a button click, without native form submission');
  ui.nodes.get('feedUrl').value = '';
  await ui.nodes.get('submitAdd').onclick();
  assert(ui.nodes.get('addError').textContent, 'empty URL must show feedback');
  assert(!ui.calls.includes('rss/addFeed'));
  ui.nodes.get('feedUrl').value = 'ftp://example.com/feed';
  await ui.nodes.get('submitAdd').onclick();
  assert(!ui.calls.includes('rss/addFeed'), 'reject unsupported URL schemes locally');
  ui.nodes.get('feedUrl').value = '  https://example.com/feed.xml  ';
  await ui.nodes.get('submitAdd').onclick();
  assert.strictEqual(ui.requests.find(item => item.method === 'rss/addFeed').params.url, 'https://example.com/feed.xml');
  assert.strictEqual(ui.nodes.get('addError').textContent, '该订阅已存在');
  assert.strictEqual(ui.nodes.get('submitAdd').disabled, false);
  let saved = [];
  const success = await boot(() => saved, { items: [], total: 0 }, async () => { const feed = { id: 'feed', title: 'Example' }; saved = [feed]; return feed; });
  success.nodes.get('feedUrl').value = 'https://example.com/rss';
  await success.nodes.get('submitAdd').onclick();
  assert.strictEqual(success.nodes.get('addDialog').hidden, true);
  assert.strictEqual(success.nodes.get('status').textContent, '订阅添加成功');
  assert.strictEqual(typeof success.nodes.get('feedUrl').onkeydown, 'function', 'Enter must have an explicit sandbox-safe handler');
  const grouped = await boot([{ id:'feed-1', title:'工程日志', url:'https://engineering.example.com/rss.xml', groupId:'group', unreadCount:4 }], { items:[], total:0 });
  const descendants = node => [node, ...(node.children || []).flatMap(descendants)];
  const feedRow = descendants(grouped.nodes.get('feeds')).find(node => node.className === 'feed-row');
  const source = descendants(feedRow).find(node => node.className === 'feed-source');
  assert(source, 'grouped feeds should show the source host separately from the title');
  assert.strictEqual(source.textContent, 'engineering.example.com');
  const html = fs.readFileSync(require('path').join(__dirname, '../ui/index.html'), 'utf8');
  assert(!/<form\b/i.test(html), 'workbench must not depend on sandbox-blocked native forms');
  console.log('PASS: empty startup, click submission, URL validation, duplicate error, successful add, and keyboard submission wiring');
})().catch(error => { console.error(error); process.exitCode = 1; });
