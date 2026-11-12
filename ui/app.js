// Apache-2.0 — RSS Reader contributors.
const bridge = window.dbxPlugin;
const $ = id => document.getElementById(id);
const defaults = {displayMode:'rich', showImages:true, imageWidth:800, density:'comfortable', timeoutSeconds:30, maxArticles:0, browser:'default',readerTheme:'auto',fontSize:17,lineHeight:1.9,readerWidth:900};
const state = { view:'feeds',categoryId:'',categories:[],libraryIndex:[],feeds: [], groups: [], collapsedGroups:new Set(), groupId: '', settings:{...defaults}, articles: [], total: 0, feedId: '', article: null, limit: 40, queryVersion: 0, refreshBusy: false, deleting: null, feedSelectionMode:false, selectedFeedIDs:new Set() };
let libraryUI,readingUI;
let searchTimer;

function status(text, error = false) {
  $('status').textContent = text;
  $('status').classList.toggle('error', error);
  const region=$('toastRegion');if(!region)return;const toast=el('div','toast'+(error?' toast-error':''),text);region.append(toast);const frame=window.requestAnimationFrame||((callback)=>setTimeout(callback,0));frame(()=>toast.classList.add('visible'));setTimeout(()=>{toast.classList.remove('visible');setTimeout(()=>{if(typeof toast.remove==='function')toast.remove();else toast.hidden=true;},220);},error?5200:3200);
}
function report(error) { status(error && error.message ? error.message : String(error || '操作失败，请重试'), true); }
function applySettings(){ document.documentElement.dataset.density=state.settings.density; }
function editGroup(group){window.RSSReading.createModal(group?'管理分组':'新建分组',m=>{
  const name=el('input');name.value=group?.name||'';name.maxLength=60;m.dialog.append(window.RSSReading.field('分组名称',name));
  m.actions.append(button('secondary','取消',()=>m.close()),button('primary','保存分组',()=>m.run(async()=>{await invoke('rss/saveGroup',{id:group?.id||'',name:name.value});m.close();await reload();})));
  if(group)m.actions.prepend(button('danger','删除分组',()=>m.run(async()=>{await invoke('rss/deleteGroup',{id:group.id});m.close();await reload();})));
});}
function moveFeed(feed){window.RSSReading.createModal('移动订阅',m=>{
  m.dialog.classList.add('move-feed-dialog');m.dialog.append(el('p','dialog-intro','选择一个分组，或创建新分组后立即移动当前订阅。'));
  const select=window.RSSReading.select([['','未分组'],...state.groups.map(g=>[g.id,g.name]),['__new__','＋ 新建分组']],feed.groupId||'');select.className='styled-select';const selectShell=el('div','select-shell');selectShell.append(select);const groupField=window.RSSReading.field('目标分组',selectShell);groupField.classList.add('group-choice-field');const newName=el('input');newName.placeholder='例如：技术、设计、稍后阅读';const newField=window.RSSReading.field('新分组名称',newName);newField.classList.add('new-group-field');newField.hidden=true;m.dialog.append(groupField,newField);
  select.onchange=()=>{newField.hidden=select.value!=='__new__';if(!newField.hidden)newName.focus();};
  m.actions.append(button('secondary','取消',()=>m.close()),button('primary','移动',()=>m.run(async()=>{let groupID=select.value;if(groupID==='__new__'){const name=newName.value.trim();if(!name)throw new Error('请输入新分组名称');const group=await invoke('rss/saveGroup',{id:'',name});if(!group?.id)throw new Error('新分组创建失败');state.groups.push(group);const option=el('option','',group.name);option.value=group.id;select.insertBefore(option,select.lastElementChild);select.value=group.id;newField.hidden=true;groupID=group.id;}await invoke('rss/moveFeed',{feedId:feed.id,groupId:groupID});m.close();await reload();})));
});}
async function invoke(method, params = {}) {
  if (!bridge || typeof bridge.invoke !== 'function') throw new Error('未连接到 DBX，请从插件工作台打开 RSS 阅读器。');
  await bridge.ready;
  return bridge.invoke(method, params, { timeoutMs: Math.max(30000, (state.settings.timeoutSeconds + 10) * 1000) });
}
function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}
function button(className, text, action, label) {
  const node = el('button', className, text);
  node.type = 'button';
  if (label) { node.title = label; node.setAttribute('aria-label', label); }
  node.onclick = () => Promise.resolve().then(action).catch(report);
  return node;
}
function formatDate(value, short = false) {
  if (!value || String(value).startsWith('0001-')) return '';
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return '';
  return short ? date.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' }) : date.toLocaleString('zh-CN');
}
function plainText(value) {
  const doc = new DOMParser().parseFromString(String(value || ''), 'text/html');
  doc.querySelectorAll('script,style').forEach(node => node.remove());
  return (doc.body.textContent || '').replace(/\s+/g, ' ').trim();
}
function safeURL(value) {
  try { const url = new URL(String(value)); return /^https?:$/.test(url.protocol) && !url.username && !url.password ? url.href : ''; }
  catch (_) { return ''; }
}
function imageSource(node, base) {
  const candidates=['data-src','data-original','data-lazy-src','data-url','src'].map(key=>node.getAttribute(key));
  const srcset=node.getAttribute('data-srcset') || node.getAttribute('srcset');
  if(srcset) candidates.push(srcset.split(',').pop().trim().split(/\s+/)[0]);
  for(const value of candidates){
    if(!value || !value.trim()) continue;
    try{const url=safeURL(new URL(value,base || undefined).href);if(url)return url;}catch(_){}
  }
  return '';
}
function sanitizedContent(value, sourceURL) {
  const doc = new DOMParser().parseFromString(String(value || ''), 'text/html');
  const allowed = new Set(['P', 'BR', 'DIV', 'SPAN', 'STRONG', 'B', 'I', 'EM', 'U', 'S', 'DEL', 'BLOCKQUOTE', 'PRE', 'CODE', 'UL', 'OL', 'LI', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'HR', 'A', 'IMG', 'TABLE', 'THEAD', 'TBODY', 'TR', 'TH', 'TD', 'FIGURE', 'FIGCAPTION', 'SUP', 'SUB']);
  doc.querySelectorAll('script,style,iframe,object,embed,form,input,button,textarea,select,link,meta,base,svg,math,template').forEach(node => node.remove());
  Array.from(doc.body.querySelectorAll('*')).forEach(node => {
    if (!allowed.has(node.tagName)) { node.replaceWith(...node.childNodes); return; }
    const rawLink = node.getAttribute('href');
    const alt = node.getAttribute('alt');
    if (node.tagName === 'IMG') {
      const url = imageSource(node,sourceURL);
      if (!state.settings.showImages || !url) { node.replaceWith(doc.createTextNode(alt ? '[' + alt + ']' : '')); return; }
      Array.from(node.attributes).forEach(attribute => node.removeAttribute(attribute.name));
      node.setAttribute('data-image-url', url); node.setAttribute('alt', alt || '文章图片');
      node.setAttribute('class','article-image image-loading');
      return;
    }
    Array.from(node.attributes).forEach(attribute => node.removeAttribute(attribute.name));
    if (node.tagName === 'A' && rawLink) {
      try { const url = safeURL(new URL(rawLink, sourceURL || undefined).href); if (url) { node.setAttribute('href', url); node.setAttribute('rel', 'noreferrer'); } } catch (_) { /* Keep link text. */ }
    }
  });
  return document.importNode(doc.body, true);
}
function articleKey(item) { return item ? item.feedId + '/' + item.id : ''; }
function feedTitle(id) { const feed = state.feeds.find(item => item.id === id); return feed ? feed.title : state.article?.feedId===id&&state.article.feedTitle?state.article.feedTitle:'订阅文章'; }

function exportOPML(feedIDs=[],filename='rss-subscriptions.opml'){
  return invoke('rss/exportOPMLFile',{feedIds:feedIDs,filename}).then(file=>{if(!file?.cancelled)status(`已导出：${String(file.filename||file.path||'').split(/[\\/]/).pop()}`);});
}

async function loadFeeds() {
  const [result, groups, library] = await Promise.all([invoke('rss/listFeeds'),invoke('rss/listGroups'),invoke('rss/library/index')]);
  state.libraryIndex=Array.isArray(library?.items)?library.items:[];state.categories=Array.isArray(library?.categories)?library.categories:[];
  if(state.categoryId&&state.categoryId!=='uncategorized'&&!state.categories.some(g=>g.id===state.categoryId))state.categoryId='';
  state.groups = Array.isArray(groups) ? groups : [];
  if (state.groupId && state.groupId !== 'ungrouped' && !state.groups.some(g => g.id === state.groupId)) state.groupId = '';
  state.feeds = Array.isArray(result) ? result.filter(item => item && typeof item.id === 'string') : [];
  state.selectedFeedIDs=new Set([...state.selectedFeedIDs].filter(id=>state.feeds.some(feed=>feed.id===id)));
  if (state.feedId && !state.feeds.some(feed => feed.id === state.feedId)) state.feedId = '';
  renderFeeds();
}
function renderFeeds() {
  const root = $('feeds'); root.replaceChildren();
  const unread = state.feeds.reduce((sum, feed) => sum + (Number(feed.unreadCount) || 0), 0);
  const all = button('nav-row' + (state.view==='feeds' && !state.feedId && !state.groupId ? ' active' : ''), '', () => selectFeed(''));
  all.append(el('span', 'nav-icon', '▤'), el('span', 'feed-name', '全部文章'), el('span', 'badge', unread));
  root.append(all);
  libraryUI?.nav(root);
  const heading = el('div', 'side-heading');
  const headingLabel=el('span','','我的订阅');
  headingLabel.append(' ',el('span','feed-count-label',`${state.feeds.length} 个订阅源`));
  const headingActions=el('div','feed-selection-tools');
  if(state.feedSelectionMode){const selected=state.selectedFeedIDs.size;headingLabel.textContent=`已选 ${selected} 个订阅`;headingActions.append(button('secondary bulk-select-all',selected===state.feeds.length?'取消全选':'全选',()=>{state.selectedFeedIDs=selected===state.feeds.length?new Set():new Set(state.feeds.map(feed=>feed.id));renderFeeds();},'全选 / 取消全选'),button('danger bulk-delete',`删除${selected?'所选':''}`,deleteSelectedFeeds,'删除所选订阅'),button('icon-button','×',()=>{state.feedSelectionMode=false;state.selectedFeedIDs=new Set();renderFeeds();},'退出批量选择'));}
  else headingActions.append(button('icon-button','☷',()=>{state.feedSelectionMode=true;renderFeeds();},'批量选择'),button('icon-button','＋',()=>editGroup(),'创建分组'));
  heading.append(headingLabel,headingActions);root.append(heading);
  if (!state.feeds.length) root.append(el('p', 'feed-placeholder', '订阅喜欢的网站，更新会汇集在这里。'));
  const appendFeed = (feed,parent=root) => {
    const row = el('div', 'feed-row' + (state.feedId === feed.id ? ' active' : ''));
    const select = button('nav-row', '', () => selectFeed(feed.id));
    select.title = [feed.title, feed.url].filter(Boolean).join('\n');
    select.setAttribute('aria-current', state.feedId === feed.id ? 'true' : 'false');
    const labels=el('span','feed-labels');labels.append(el('span','feed-name',feed.title||feed.url||'未命名订阅'));
    try{const sourceHost=new URL(feed.url).hostname;if(sourceHost)labels.append(el('span','feed-source',sourceHost));}catch(_){}
    select.append(labels);
    if (feed.unreadCount) select.append(el('span', 'badge', feed.unreadCount));
    const actions = el('div', 'feed-actions');
    const refresh = button('icon-button', '↻', async () => {
      refresh.disabled = true;
      try { status('正在刷新 ' + feed.title + '…'); await invoke('rss/refreshFeed', { feedId: feed.id }); await reload(); status('订阅已更新'); }
      finally { refresh.disabled = false; }
    }, '刷新 ' + feed.title);
    if(!state.feedSelectionMode)actions.append(refresh, button('icon-button','↗',()=>exportOPML([feed.id],'rss-subscription.opml'),'分享 / 导出 '+feed.title),button('icon-button','⇄',()=>moveFeed(feed),'移动到分组'), button('icon-button', '×', () => openDelete(feed), '删除 ' + feed.title));
    if(state.feedSelectionMode){row.classList.add('selection-mode');const check=el('input','feed-select');check.type='checkbox';check.checked=state.selectedFeedIDs.has(feed.id);check.setAttribute('aria-label','选择 '+(feed.title||feed.url));check.onchange=()=>{if(check.checked)state.selectedFeedIDs.add(feed.id);else state.selectedFeedIDs.delete(feed.id);renderFeeds();};row.append(check);}
    row.append(select, actions); parent.append(row);
  };
  [...state.groups,{id:'ungrouped',name:'未分组'}].forEach(group=>{
    const members=state.feeds.filter(f=>(f.groupId || 'ungrouped')===group.id);
    const row=el('div','group-heading');
    const unread=members.reduce((n,f)=>n+(f.unreadCount||0),0);
    const expanded=!state.collapsedGroups.has(group.id);const toggleGroup=()=>{if(state.collapsedGroups.has(group.id))state.collapsedGroups.delete(group.id);else state.collapsedGroups.add(group.id);renderFeeds();};const groupButton=button('nav-row group-toggle'+(state.groupId===group.id?' active':''),'',()=>{toggleGroup();if(state.groupId!==group.id)selectGroup(group.id).catch(report);});groupButton.setAttribute('aria-expanded',String(expanded));groupButton.append(el('span','group-name',group.name),el('span','group-unread',String(unread)));row.append(groupButton);
    if(group.id!=='ungrouped'){if(members.length)row.append(button('icon-button','↗',()=>exportOPML(members.map(f=>f.id),'rss-group.opml'),'分享 / 导出 '+group.name));row.append(button('icon-button','⋯',()=>editGroup(group),'管理分组'));}
    const memberList=el('div','group-feed-list');if(expanded)members.forEach(feed=>appendFeed(feed,memberList));root.append(row,memberList);
  });
  $('feedCount').textContent=state.feeds.length+' 个订阅源';
  $('viewTitle').textContent = scopeTitle();
  $('markAllRead').hidden=state.view!=='feeds';$('unreadOnly').disabled=state.view!=='feeds';$('viewAll').disabled=state.view!=='feeds';$('viewUnread').disabled=state.view!=='feeds';
}
function scopeTitle(){if(state.view==='favorites')return state.categories.find(g=>g.id===state.categoryId)?.name||(state.categoryId==='uncategorized'?'未分类收藏':'我的收藏');return state.feedId?feedTitle(state.feedId):state.groupId==='ungrouped'?'未分组':state.groups.find(g=>g.id===state.groupId)?.name||'全部文章';}
async function selectGroup(id){await selectFeed('',id);}
async function selectFeed(id, groupId = '') {
  state.view='feeds';state.categoryId='';state.feedId = id; state.groupId = groupId; state.limit = 40; state.article = null;
  document.body.classList.remove('reading','focus-reading'); renderFeeds(); clearReader(); await loadArticles();
}
async function loadArticles() {
  const version = ++state.queryVersion;
  $('articles').setAttribute('aria-busy', 'true');
  try {
    let result;
    if(state.view==='feeds')result=await invoke('rss/listArticles', { feedId: state.feedId, groupId:state.groupId, unread: $('unreadOnly').checked, search: $('search').value.trim(), limit: state.limit });
    else {
      const query={favorites:state.view==='favorites',categoryId:state.categoryId,search:$('search').value.trim()};const limit=state.limit;result={items:[],total:0};
      for(let offset=0;offset<limit;offset+=200){
        const page=await invoke('rss/library/list',{...query,offset,limit:Math.min(200,limit-offset)});if(version!==state.queryVersion)return;
        const items=Array.isArray(page?.items)?page.items:[];result.items.push(...items);result.total=Number(page?.total)||0;if(!items.length||result.items.length>=result.total)break;
      }
    }
    if (version !== state.queryVersion) return;
    state.articles = result && Array.isArray(result.items) ? result.items.filter(item => item && item.id) : [];
    state.total = result && Number.isFinite(result.total) ? result.total : state.articles.length;
    renderArticles();
  } finally { if (version === state.queryVersion) $('articles').setAttribute('aria-busy', 'false'); }
}
function renderArticles() {
  ['viewAll', 'viewUnread'].forEach((id, index) => {
    const active = $('unreadOnly').checked === Boolean(index);
    $(id).classList.toggle('active', active);
    $(id).setAttribute('aria-selected', String(active));
  });
  const root = $('articles'); root.replaceChildren();
  $('articleCount').textContent = state.total + ' 篇文章';
  $('listOrder').textContent=state.view==='favorites'?'最近收藏优先':'最新发布优先';
  $('empty').hidden = state.articles.length > 0;
  $('loadMore').hidden = state.articles.length >= state.total;
  if (!state.articles.length) {
    const hasFeeds = state.feeds.length > 0;
    const filtered = $('search').value.trim() || $('unreadOnly').checked;
    $('emptyTitle').textContent = hasFeeds ? (filtered ? '没有匹配的文章' : '订阅已就位，等待新内容') : '开始你的第一份订阅';
    $('emptyText').textContent = hasFeeds ? (filtered ? '试试其他关键词，或关闭“仅未读”。' : '点击刷新订阅，获取这个来源的最新文章。') : '添加 RSS 或 Atom 地址，让喜欢的内容在这里相遇。';
    $('emptyAdd').hidden = hasFeeds;
    if(state.view==='favorites'){$('emptyTitle').textContent='还没有收藏文章';$('emptyText').textContent='在文章中点击收藏。';$('emptyAdd').hidden=true;}
  }
  state.articles.forEach(item => {
    const row = button('article' + (item.isRead ? '' : ' unread') + (articleKey(state.article) === articleKey(item) ? ' active' : ''), '', () => openArticle(item));
    const meta = el('div', 'article-source'); meta.append(el('span', '', item.feedTitle||feedTitle(item.feedId)), el('time', '', formatDate(item.publishedAt, true)));
    row.append(meta, el('div', 'title', item.title || '无标题'), el('div', 'summary', plainText(item.summary || item.content).slice(0, 240) || '点击阅读文章'));
    const card=el('div','article-card');card.append(row);if(libraryUI)card.append(libraryUI.articleAction(item));root.append(card);
  });
}
function clearReader() {
  $('reader').innerHTML = '<div class="reader-welcome"><div class="welcome-mark" aria-hidden="true">▤</div><h2>选择文章，开始阅读</h2><p>点击左侧文章查看内容。<br />阅读记录会自动保存在本机。</p><div class="reader-hint"><strong>订阅小提示</strong><span>支持 RSS 2.0、RSS 1.0 / RDF 和 Atom 1.0。请添加网站的订阅源地址。</span></div></div>';
}
async function copyLink(url) {
  const safe = safeURL(url); if (!safe) throw new Error('这篇文章没有有效的原文链接');
  if (!navigator.clipboard || !navigator.clipboard.writeText) throw new Error('当前环境不支持复制链接');
  await navigator.clipboard.writeText(safe); status('原文链接已复制，可粘贴到浏览器打开');
}
async function openLink(url) { const safe=safeURL(url); if(!safe) throw new Error('这篇文章没有有效的原文链接'); await invoke('rss/openURL',{url:safe}); status('已在电脑浏览器打开'); }
function renderReader(item) {
  const root = $('reader'); root.replaceChildren();
  const tools = el('div', 'reader-tools');
  const back=button('secondary', '← 返回列表', () => { document.body.classList.remove('reading','focus-reading'); $('reader').scrollTop = 0; });back.classList.add('reader-back-button');tools.append(back);
  const actions = el('div', 'reader-actions');
  const read = button('secondary', item.isRead ? '标为未读' : '标为已读', async () => {
    read.disabled = true;
    try { await invoke('rss/setRead', { feedId: item.feedId, articleId: item.id, read: !item.isRead }); item.isRead = !item.isRead; renderReader(item); await reload(); }
    finally { read.disabled = false; }
  });
  actions.append(read);
  if (safeURL(item.link)) actions.append(button('secondary', '浏览器打开 ↗', () => openLink(item.link)));
  tools.append(actions); root.append(tools);
  readingUI?.mount(tools);
  const content = el('div', 'reading-content');
  content.append(el('div', 'reader-source', feedTitle(item.feedId)), el('h2', '', item.title || '无标题'));
  content.append(el('div', 'reader-meta', [item.author, formatDate(item.publishedAt)].filter(Boolean).join(' · ') || '暂无作者与发布时间'));
  const imageToggle=button('secondary image-toggle',state.settings.showImages?'隐藏图片':'显示图片',async()=>{
    const settings={...state.settings,showImages:!state.settings.showImages};
    await invoke('rss/saveSettings',settings);state.settings=settings;applySettings();renderReader(item);
  });content.append(imageToggle);
  const body = el('div', 'body');
  body.style.setProperty('--article-image-width',state.settings.imageWidth+'px');
  if (item.content || item.summary) {
    const value=item.content||item.summary;
    const parsed = sanitizedContent(value, item.link || state.feeds.find(f=>f.id===item.feedId)?.url);body.append(...parsed.childNodes);
    body.querySelectorAll('a').forEach(link => { link.onclick = event => { event.preventDefault(); if (link.href) openLink(link.href).catch(report); }; });
  } else {
    const empty=el('div','article-empty');empty.append(el('p','', '订阅源未在 RSS 中提供正文，正在尝试获取原文页面。'));
    const retry=button('secondary','获取原文正文',async()=>{retry.disabled=true;retry.textContent='正在获取…';try{const result=await invoke('rss/fetchArticle',{url:item.link});if(!result?.content)throw new Error('原文页面中未找到可提取的正文');await invoke('rss/saveArticleContent',{feedId:item.feedId,articleId:item.id,content:result.content});item.content=result.content;renderReader(item);}catch(error){retry.disabled=false;retry.textContent='重试获取正文';empty.querySelector('p').textContent=error.message||'原文正文获取失败';}});empty.append(retry);body.append(empty);
    if(!item._contentAttempted && safeURL(item.link)){item._contentAttempted=true;retry.click();}
  }
  if(state.settings.showImages && safeURL(item.imageUrl) && !body.querySelector('img')){
    const image=el('img','article-image image-loading');image.setAttribute('data-image-url',safeURL(item.imageUrl));image.alt='文章配图';body.prepend(image);
  }
  content.append(body); root.append(content);
  libraryUI?.mountReader(item,tools,content,body);
  hydrateImages(body);
}
let imageQueue = [], imageActive = 0, imageObserver;
function hydrateImages(body){
  if(imageObserver)imageObserver.disconnect();imageQueue=[];
  const enqueue=image=>{imageQueue.push({image,body});pumpImages();};
  const images=Array.from(body.querySelectorAll('img[data-image-url]')).slice(0,60);
  // Load the first images immediately: an img without src may have zero size
  // and never intersect. Remaining images have a CSS placeholder for lazy load.
  images.slice(0,6).forEach(enqueue);
  if(typeof IntersectionObserver==='function'){
    imageObserver=new IntersectionObserver(entries=>entries.forEach(entry=>{if(entry.isIntersecting){imageObserver.unobserve(entry.target);enqueue(entry.target);}}),{root:$('reader'),rootMargin:'480px'});
    images.slice(6).forEach(img=>imageObserver.observe(img));
  }else images.slice(6).forEach(enqueue);
}
function pumpImages(){
  while(imageActive<6 && imageQueue.length){
    const {image,body}=imageQueue.shift();if(!body.isConnected)continue; imageActive++;
    invoke('rss/fetchImage',{url:image.getAttribute('data-image-url')}).then(result=>{
      if(body.isConnected && state.settings.showImages){
        if(!/^data:image\/(png|jpeg|gif|webp);base64,/.test(result?.dataURL||''))throw new Error('图片服务返回数据无效');
        image.onload=()=>image.classList.remove('image-loading');
        image.onerror=()=>imageFailure(image,'图片解码失败或被宿主限制');
        image.src=result.dataURL;
      }
    }).catch(error=>{if(body.isConnected)imageFailure(image,error.message);}).finally(()=>{imageActive--;pumpImages();});
  }
}
function imageFailure(image,reason){
  image.classList.remove('image-loading');image.classList.add('image-failed');
  image.alt='图片加载失败：'+(reason||'请稍后重试');image.title=image.alt;
}
async function openArticle(item) {
  state.article = item;
  document.body.classList.add('reading'); renderReader(item); renderArticles(); $('reader').scrollTop = 0;
  if (!item.isRead) {
    await invoke('rss/setRead', { feedId: item.feedId, articleId: item.id, read: true });
    item.isRead = true;
    // Only repaint the reader if this article is still selected.
    if (articleKey(state.article) === articleKey(item)) renderReader(item);
    await reload();
  }
}
async function reload() { await loadFeeds(); await loadArticles(); }
function openAdd() { const select=$('addGroupSelect');const option=(label,value)=>{const node=document.createElement('option');node.textContent=label;node.value=value;return node;};select.replaceChildren(option('未分组',''),...state.groups.map(group=>option(group.name,group.id)),option('＋ 新建分组','__new__'));$('addNewGroupField').hidden=true;$('addNewGroup').hidden=true;$('addNewGroup').value='';$('addDialog').hidden = false; $('addError').textContent = ''; $('feedUrl').focus(); }
function closeAdd() { if (!$('submitAdd').disabled) { $('addDialog').hidden = true; $('addFeed').focus(); } }
function openDelete(feed) { state.deleting = feed; $('deleteText').textContent = '将移除“' + feed.title + '”及其本地文章和阅读记录。'; $('deleteDialog').hidden = false; $('cancelDelete').focus(); }
function deleteSelectedFeeds() { const ids=[...state.selectedFeedIDs]; if(!ids.length){status('请先选择要删除的订阅',true);return;} state.deleting={bulk:true,ids,title:`${ids.length} 个订阅`};$('deleteText').textContent=`将移除已选择的 ${ids.length} 个订阅及其本地文章和阅读记录。`;$('deleteDialog').hidden=false;$('cancelDelete').focus(); }
function closeDelete() { if (!$('confirmDelete').disabled) { state.deleting = null; $('deleteDialog').hidden = true; $('addFeed').focus(); } }
$('addFeed').onclick = $('emptyAdd').onclick = openAdd;
$('importOPML').onclick=()=>$('opmlFile').click();
$('opmlFile').onchange=async()=>{const file=$('opmlFile').files?.[0];if(!file)return;try{if(file.size>2*1024*1024)throw new Error('OPML 文件超过 2 MiB');const content=await file.text();const feeds=await invoke('rss/previewOPML',{content});window.RSSReading.createModal('导入订阅预览',m=>{const list=el('div','opml-preview');list.append(el('p','',`发现 ${feeds.length} 个订阅地址，重复订阅将跳过。`));feeds.forEach(feed=>list.append(el('div','opml-preview-row',`${feed.group?feed.group+' / ':''}${feed.title||feed.url} · ${feed.url}`)));m.dialog.append(list);m.actions.append(button('secondary','取消',()=>m.close()),button('primary','导入订阅',()=>m.run(async()=>{const started=await invoke('rss/importOPMLStart',{content});const progress=el('div','import-progress');const label=el('p','import-progress-label',`准备导入 0 / ${started.total}`);const bar=document.createElement('progress');bar.max=started.total;bar.value=0;progress.append(label,bar);m.dialog.append(progress);let result;for(;;){await new Promise(resolve=>setTimeout(resolve,500));const update=await invoke('rss/importOPMLStatus',{jobId:started.jobId});bar.value=Number(update.current)||0;label.textContent=update.url?`正在获取 ${update.current} / ${update.total}：${update.url}`:`正在处理 ${update.current} / ${update.total}`;if(update.error)throw new Error(update.error);if(update.done){result=update.result||{};break;}}m.close();await reload();status(`导入完成：新增 ${(result.added||[]).length}，跳过 ${(result.skipped||[]).length}，失败 ${(result.failed||[]).length}${result.failed?.length?'；'+result.failed.map(x=>x.url+'：'+x.error).join('；'):''}`,!!result.failed?.length);})));});}catch(e){report(e);}finally{$('opmlFile').value='';}};
$('exportOPML').onclick=()=>exportOPML();
$('settingsButton').onclick = async()=>{try{const [settings,browsers]=await Promise.all([invoke('rss/getSettings'),invoke('rss/listBrowsers')]);state.settings={...defaults,...settings,displayMode:'rich'};$('showImages').checked=state.settings.showImages;$('imageWidth').value=state.settings.imageWidth;$('density').value=state.settings.density;$('timeoutSeconds').value=state.settings.timeoutSeconds;$('maxArticles').value=state.settings.maxArticles;$('browserChoice').replaceChildren(...(Array.isArray(browsers)?browsers:[]).map(b=>{const o=document.createElement('option');o.value=b.id;o.textContent=b.name;return o;}));$('browserChoice').value=state.settings.browser;$('settingsDialog').hidden=false;}catch(e){report(e);}};
function closeSettings(){$('settingsDialog').hidden=true;}
$('closeSettings').onclick=$('cancelSettings').onclick=closeSettings;
$('saveSettings').onclick=async()=>{const settings={...state.settings,displayMode:'rich',showImages:$('showImages').checked,imageWidth:Number($('imageWidth').value),density:$('density').value,timeoutSeconds:Number($('timeoutSeconds').value),maxArticles:Number($('maxArticles').value),browser:$('browserChoice').value};try{await invoke('rss/saveSettings',settings);state.settings=settings;applySettings();closeSettings();if(state.article)renderReader(state.article);status('设置已保存');}catch(e){$('settingsError').textContent=e.message;}};
$('addGroupSelect').onchange=()=>{const create=$('addGroupSelect').value==='__new__';$('addNewGroupField').hidden=!create;$('addNewGroup').hidden=!create;if(create)$('addNewGroup').focus();};
$('markAllRead').onclick=async()=>{try{const result=await invoke('rss/markAllRead',{feedId:state.feedId,groupId:state.groupId});await reload();status(`已将 ${result.count||0} 篇文章标为已读`);}catch(e){report(e);}};
$('closeDialog').onclick = $('cancelAdd').onclick = closeAdd;
$('cancelDelete').onclick = closeDelete;
async function submitFeed() {
  if ($('submitAdd').disabled) return;
  const url = $('feedUrl').value.trim();
  if (!safeURL(url)) {
    $('addError').textContent = url ? '请输入有效的 HTTP 或 HTTPS 订阅地址。' : '请先输入 RSS 或 Atom 订阅地址。';
    $('feedUrl').focus();
    return;
  }
  $('submitAdd').disabled = true; $('submitAdd').textContent = '正在获取…'; $('addError').textContent = '';
  try {
    const feed = await invoke('rss/addFeed', { url });
    if (!feed || !feed.id) throw new Error('订阅服务未返回有效数据，请重试');
    let groupID=$('addGroupSelect').value;if(groupID==='__new__'){const name=$('addNewGroup').value.trim();if(!name)throw new Error('请输入新分组名称');const group=await invoke('rss/saveGroup',{id:'',name});if(!group?.id)throw new Error('新分组创建失败');groupID=group.id;}if(groupID)await invoke('rss/moveFeed',{feedId:feed.id,groupId:groupID});
    state.view='feeds';state.feedId = feed.id; state.groupId=''; state.limit = 40; state.article = null; clearReader(); document.body.classList.remove('reading','focus-reading');
    $('feedUrl').value = ''; $('addDialog').hidden = true; $('addFeed').focus();
    await reload(); status('订阅添加成功');
  } catch (error) { $('addError').textContent = error.message || '添加失败，请检查订阅地址'; report(error); }
  finally { $('submitAdd').disabled = false; $('submitAdd').textContent = '添加订阅'; }
}
// Native form submission is blocked by DBX's allow-scripts-only iframe.
$('submitAdd').onclick = submitFeed;
$('feedUrl').onkeydown = event => {
  if (event.key === 'Enter' && !event.isComposing) { event.preventDefault(); submitFeed().catch(report); }
};
$('confirmDelete').onclick = async () => {
  if (!state.deleting || $('confirmDelete').disabled) return;
  const feed = state.deleting; $('confirmDelete').disabled = true;
  try {
    const ids=feed.bulk?feed.ids:[feed.id];
    await invoke(feed.bulk?'rss/deleteFeeds':'rss/deleteFeed', feed.bulk?{feedIds:ids}:{feedId:feed.id});
    if (ids.includes(state.feedId)) state.feedId = '';
    if (state.article && ids.includes(state.article.feedId)) { state.article = null; clearReader(); document.body.classList.remove('reading'); }
    state.selectedFeedIDs=new Set();state.feedSelectionMode=false;state.deleting = null; $('deleteDialog').hidden = true; await reload(); status('订阅已删除'); $('addFeed').focus();
  } catch (error) { report(error); }
  finally { $('confirmDelete').disabled = false; }
};
$('refreshAll').onclick = async () => {
  if (state.refreshBusy) return;
  state.refreshBusy = true; $('refreshAll').disabled = true;
  try {
    await loadFeeds();
    if (!state.feeds.length) { status('先添加一个订阅源，再获取最新内容'); return; }
    const failures = [];
    for (let i = 0; i < state.feeds.length; i++) {
      const feed = state.feeds[i]; status('正在刷新 ' + (i + 1) + '/' + state.feeds.length + ' · ' + feed.title);
      try { await invoke('rss/refreshFeed', { feedId: feed.id }); } catch (error) { failures.push(feed.title+'：'+error.message); }
    }
    await reload();
    status(failures.length ? '部分来源刷新失败：' + failures.join('、') : '全部订阅已更新', failures.length > 0);
  } catch (error) { report(error); }
  finally { state.refreshBusy = false; $('refreshAll').disabled = false; }
};
$('search').oninput = () => { clearTimeout(searchTimer); searchTimer = setTimeout(() => { state.limit = 40; loadArticles().catch(report); }, 250); };
$('unreadOnly').onchange = () => { state.limit = 40; loadArticles().catch(report); };
['viewAll', 'viewUnread'].forEach((id, index) => {
  $(id).onclick = () => { $('unreadOnly').checked = Boolean(index); state.limit = 40; loadArticles().catch(report); };
});
$('loadMore').onclick = async () => { $('loadMore').disabled = true; state.limit += 40; try { await loadArticles(); } catch (error) { report(error); } finally { $('loadMore').disabled = false; } };
document.addEventListener('keydown', event => {
  const modal = document.querySelector('.modal-backdrop:not([hidden])');
  if (event.key === 'Escape') {
    if(window.RSSReading.closeModal()){event.preventDefault();return;}
    if(modal){closeAdd();closeDelete();closeSettings();return;}
    if(document.body.classList.contains('focus-reading')){readingUI.exit();return;}document.body.classList.remove('reading');
  }
  if (event.key === 'Tab' && modal) {
    const nodes = Array.from(modal.querySelectorAll('button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled)'));
    const first = nodes[0], last = nodes[nodes.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
  }
});
function applyTheme(theme) {
  const mode = theme && theme.appearance;
  if (mode === 'dark' || mode === 'light') document.documentElement.dataset.dbxTheme = mode;
}
window.addEventListener('dbx-plugin-env', event => applyTheme(event.detail && event.detail.theme));
async function initialize() {
  if (bridge) { await bridge.ready; applyTheme(bridge.theme); }
  state.settings={...defaults,...await invoke('rss/getSettings'),displayMode:'rich'};applySettings();
  await reload(); status(state.feeds.length ? '订阅已加载，选择文章开始阅读' : '欢迎，添加你的第一个订阅源');
}
const readerAPI={state,el,button,invoke,loadFeeds,loadArticles,renderReader,report,clearReader};
readingUI=window.RSSReading.create(readerAPI);libraryUI=window.RSSLibrary.create(readerAPI);
initialize().catch(error => { report(error); $('empty').hidden = false; $('emptyTitle').textContent = '暂时无法加载'; $('emptyText').textContent = '点击刷新重试，并确认插件后端运行正常。'; $('emptyAdd').hidden = true; });
