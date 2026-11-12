// Apache-2.0
(function(){
  let currentModal;
  function createModal(title,build){
    if(currentModal)currentModal.close();
    const previous=document.activeElement;
    const backdrop=document.createElement('div');backdrop.className='modal-backdrop generated-modal';
    const dialog=document.createElement('section');dialog.className='dialog library-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label',title);
    const h=document.createElement('h2');h.textContent=title;dialog.append(h);
    const error=document.createElement('p');error.className='error-text';error.setAttribute('role','alert');
    const actions=document.createElement('div');actions.className='dialog-actions';
    const modal={dialog,actions,error,close(){backdrop.remove();if(currentModal===modal)currentModal=null;previous?.focus();},async run(action){if(modal.busy)return;modal.busy=true;error.textContent='';const buttons=Array.from(dialog.querySelectorAll('button'));buttons.forEach(b=>b.disabled=true);try{await action();}catch(e){error.textContent=e.message||String(e);}finally{modal.busy=false;buttons.forEach(b=>b.disabled=false);}}};
    currentModal=modal;build(modal);dialog.append(error,actions);backdrop.append(dialog);document.body.append(backdrop);
    dialog.querySelector('input,textarea,select,button')?.focus();return modal;
  }
  function field(title,input){const label=document.createElement('label');label.className='editor-field';const span=document.createElement('span');span.textContent=title;label.append(span,input);return label;}
  function select(options,value){const node=document.createElement('select');options.forEach(([id,name])=>{const o=document.createElement('option');o.value=id;o.textContent=name;node.append(o);});node.value=value;return node;}
  function create(api){
    const {el,button}=api;
    function exit(){document.body.classList.remove('focus-reading');const focus=document.getElementById('focusButton');if(focus){focus.textContent='专注阅读';focus.focus();}}
    function mount(tools){
      const actions=el('div','reader-preferences');const focus=button('secondary',document.body.classList.contains('focus-reading')?'退出专注':'专注阅读',()=>{const active=document.body.classList.toggle('focus-reading');focus.textContent=active?'退出专注':'专注阅读';});focus.id='focusButton';
      actions.append(focus);tools.append(actions);
    }
    return {mount,exit};
  }
  window.RSSReading={create,createModal,field,select,closeModal(){if(currentModal){if(!currentModal.busy)currentModal.close();return true;}return false;}};
})();
