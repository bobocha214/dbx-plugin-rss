// Apache-2.0
(function(){
  function create(api){
    const {state,el,button,invoke,loadFeeds,loadArticles,renderReader,report,clearReader}=api;
    const {createModal,field,select}=window.RSSReading;
    function info(item){return state.libraryIndex.find(a=>a.feedId===item.feedId&&a.articleId===item.id)||{};}
    async function refresh(){await loadFeeds();await loadArticles();}
    async function choose(view,category=''){
      state.view=view;state.categoryId=category;state.feedId='';state.groupId='';state.limit=40;state.article=null;document.body.classList.remove('reading','focus-reading');clearReader();await refresh();
    }
    function nav(root){
      const count=state.libraryIndex.filter(a=>a.isFavorite).length;
      root.append(button('nav-row'+(state.view==='favorites'&&!state.categoryId?' active':''),'★ 我的收藏 · '+count,()=>choose('favorites')));
      if(state.view==='favorites'){
        root.append(button('nav-row category-row'+(state.categoryId==='uncategorized'?' active':''),'未分类',()=>choose('favorites','uncategorized')));
        state.categories.forEach(g=>{const row=el('div','category-heading');row.append(button('nav-row category-row'+(state.categoryId===g.id?' active':''),g.name,()=>choose('favorites',g.id)),button('icon-button','⋯',()=>editCategory(g),'管理收藏分类 '+g.name));root.append(row);});
        root.append(button('nav-row category-row','＋ 新建收藏分类',()=>editCategory()));
      }
    }
    function editCategory(g){createModal(g?'管理收藏分类':'新建收藏分类',m=>{
      const input=el('input');input.value=g?.name||'';input.maxLength=60;m.dialog.append(field('分类名称',input));
      m.actions.append(button('secondary','取消',()=>m.close()),button('primary','保存分类',()=>m.run(async()=>{await invoke('rss/library/saveCategory',{id:g?.id||'',name:input.value});m.close();await refresh();if(state.article)renderReader(state.article);})));
      if(g)m.actions.prepend(button('danger','删除分类',()=>m.run(async()=>{await invoke('rss/library/deleteCategory',{id:g.id});if(state.categoryId===g.id)state.categoryId='';m.close();await refresh();if(state.article)renderReader(state.article);})));
      if(g)m.dialog.append(el('p','','删除分类会将收藏移回“未分类”，文章仍会保留。'));
    });}
    async function favorite(item){const v=info(item);await invoke('rss/library/setFavorite',{feedId:item.feedId,articleId:item.id,favorite:!v.isFavorite,categoryId:v.categoryId||''});await refresh();if(state.article&&state.article.id===item.id&&state.article.feedId===item.feedId)renderReader(state.article);}
    function articleAction(item){const v=info(item);const action=button('favorite-button'+(v.isFavorite?' saved':''),v.isFavorite?'★':'☆',()=>favorite(item),v.isFavorite?'取消收藏 '+item.title:'收藏 '+item.title);action.setAttribute('aria-pressed',String(!!v.isFavorite));return action;}
    function mountReader(item,tools,content,body){
      const actions=el('div','library-reader-actions');actions.append(articleAction(item));
      if(info(item).isFavorite){
        const category=select([['','未分类'],...state.categories.map(g=>[g.id,g.name])],info(item).categoryId||'');category.setAttribute('aria-label','文章收藏分类');
        category.onchange=()=>invoke('rss/library/setFavorite',{feedId:item.feedId,articleId:item.id,favorite:true,categoryId:category.value}).then(refresh).catch(report);actions.append(category);
        actions.append(button('secondary','新建分类',()=>editCategory()));
      }
    }
    return {nav,articleAction,mountReader,choose};
  }
  window.RSSLibrary={create};
})();
