(() => {
  'use strict';

  if (document.getElementById('workActivityDialog')) return;
  const style = document.createElement('style');
  style.textContent = `
    #workActivityDialog{position:fixed;inset:0;z-index:10030;display:none;align-items:center;justify-content:center;padding:18px;background:#020617aa;backdrop-filter:blur(5px);font-family:Manrope,system-ui,sans-serif;color:#e8eefc}
    #workActivityDialog.open{display:flex}
    .work-activity-card{display:flex;flex-direction:column;width:min(720px,100%);max-height:min(84vh,820px);overflow:hidden;border:1px solid #425474;border-radius:24px;background:#111b2e;box-shadow:0 26px 90px #020617cc}
    .work-activity-head{display:flex;justify-content:space-between;align-items:flex-start;gap:16px;padding:20px 22px;border-bottom:1px solid #2b3953;background:linear-gradient(115deg,#202c4c,#153b43)}
    .work-activity-kicker{font-size:10px;font-weight:800;letter-spacing:.17em;color:#69ddcd}
    .work-activity-title{margin-top:4px;font-size:19px;font-weight:800;color:#f7f9ff}
    .work-activity-subtitle{margin-top:3px;font-size:12px;color:#aab8cf}
    .work-activity-close{width:38px;height:38px;border:1px solid #5b6a85;border-radius:12px;background:#101a2c;color:#f8fafc;font-size:20px;cursor:pointer}
    .work-attachment-panel{padding:13px 22px;border-bottom:1px solid #2b3953;background:#121d30}
    .work-attachment-head{display:flex;align-items:center;justify-content:space-between;gap:12px}
    .work-attachment-title{font-size:12px;font-weight:800;color:#e4ecfb}
    .work-attachment-hint{margin-top:3px;font-size:10px;color:#91a2bc}
    .work-attachment-add{flex:none;border:1px solid #3c9b91;border-radius:10px;background:#113b3a;padding:8px 11px;color:#8ef0d7;font-size:11px;font-weight:800;cursor:pointer}
    .work-attachment-add:disabled{opacity:.55;cursor:wait}
    .work-attachment-list{display:flex;flex-wrap:wrap;gap:7px;margin-top:10px}
    .work-attachment-link{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;border:1px solid #334663;border-radius:9px;background:#18263c;padding:7px 9px;color:#bed8ff;text-decoration:none;font-size:11px}
    .work-attachment-empty{margin:8px 0 0;color:#8697b1;font-size:11px}
    .work-attachment-error{margin:8px 0 0;color:#fda4af;font-size:11px}
    .work-activity-list{flex:1;min-height:130px;overflow:auto;padding:16px 22px}
    .work-activity-entry{position:relative;margin:0 0 14px 7px;padding:0 0 0 20px;border-left:1px solid #3c4d6c}
    .work-activity-entry:before{position:absolute;left:-5px;top:4px;width:9px;height:9px;border:2px solid #111b2e;border-radius:50%;background:#38cdb6;content:""}
    .work-activity-meta{display:flex;flex-wrap:wrap;align-items:center;gap:7px;font-size:11px;color:#a8b6ce}
    .work-activity-actor{font-weight:800;color:#eff4ff}
    .work-activity-role{padding:3px 7px;border-radius:999px;background:#202d44;color:#a9b7cd;font-size:9px;font-weight:800;text-transform:uppercase;letter-spacing:.04em}
    .work-activity-kind{padding:3px 7px;border-radius:999px;background:#253753;color:#bdece6;font-size:9px;font-weight:800;text-transform:uppercase;letter-spacing:.04em}
    .work-activity-message{margin-top:5px;color:#d3dced;font-size:13px;line-height:1.5;white-space:pre-wrap;overflow-wrap:anywhere}
    .work-activity-empty{padding:30px 12px;text-align:center;color:#9cabc2;font-size:13px}
    .work-activity-composer{padding:14px 18px 18px;border-top:1px solid #2b3953;background:#141f33}
    .work-activity-composer label{display:block;margin-bottom:7px;font-size:11px;font-weight:800;color:#c4d1e4}
    .work-activity-composer textarea{display:block;width:100%;min-height:88px;resize:vertical;border:1px solid #3b4c6b;border-radius:14px;background:#0f192a;padding:11px 12px;color:#f3f6ff;font:500 13px/1.5 Manrope,system-ui,sans-serif;outline:none}
    .work-activity-composer textarea:focus{border-color:#7982ff;box-shadow:0 0 0 3px #6467ee33}
    .work-activity-footer{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-top:10px}
    .work-activity-error{min-height:18px;color:#fda4af;font-size:11px}
    .work-activity-submit{border:0;border-radius:11px;background:linear-gradient(100deg,#6064ef,#257bb9);padding:10px 16px;color:white;font-size:12px;font-weight:800;cursor:pointer}
    .work-activity-submit:disabled{opacity:.55;cursor:wait}
    @media(max-width:520px){#workActivityDialog{padding:8px}.work-activity-card{max-height:90vh;border-radius:18px}.work-activity-head{padding:16px}.work-activity-list{padding:14px}.work-activity-composer{padding:12px}}
  `;
  document.head.appendChild(style);

  const dialog = document.createElement('div');
  dialog.id = 'workActivityDialog';
  dialog.setAttribute('role', 'dialog');
  dialog.setAttribute('aria-modal', 'true');
  dialog.setAttribute('aria-labelledby', 'workActivityTitle');
  dialog.innerHTML = `
    <section class="work-activity-card">
      <header class="work-activity-head">
        <div><div class="work-activity-kicker">WORK HISTORY</div><h2 id="workActivityTitle" class="work-activity-title">Activity</h2><p class="work-activity-subtitle">Internal notes and changes</p></div>
        <button class="work-activity-close" type="button" aria-label="Close work history">×</button>
      </header>
      <section class="work-attachment-panel" aria-label="Attachments">
        <div class="work-attachment-head"><div><div class="work-attachment-title">Files</div><div class="work-attachment-hint">PDF, images, TXT or CSV · up to 10 MB</div></div><button class="work-attachment-add" id="workAttachmentButton" type="button">Add file</button><input id="workAttachmentInput" type="file" accept=".pdf,.png,.jpg,.jpeg,.gif,.webp,.txt,.csv" hidden></div>
        <div class="work-attachment-list" id="workAttachmentList" aria-live="polite"></div>
        <p class="work-attachment-error" id="workAttachmentError" role="status"></p>
      </section>
      <div class="work-activity-list" id="workActivityList" aria-live="polite"></div>
      <form class="work-activity-composer" id="workActivityForm">
        <label for="workActivityMessage">Add an internal note</label>
        <textarea id="workActivityMessage" maxlength="2000" placeholder="Write an update for the team…" required></textarea>
        <div class="work-activity-footer"><p class="work-activity-error" id="workActivityError" role="status"></p><button class="work-activity-submit" id="workActivitySubmit" type="submit">Add note</button></div>
      </form>
    </section>`;
  document.body.appendChild(dialog);

  const list = document.getElementById('workActivityList');
  const form = document.getElementById('workActivityForm');
  const message = document.getElementById('workActivityMessage');
  const error = document.getElementById('workActivityError');
  const submit = document.getElementById('workActivitySubmit');
  const attachmentButton = document.getElementById('workAttachmentButton');
  const attachmentInput = document.getElementById('workAttachmentInput');
  const attachmentList = document.getElementById('workAttachmentList');
  const attachmentError = document.getElementById('workAttachmentError');
  const title = document.getElementById('workActivityTitle');
  let activeWork = null;
  let previousFocus = null;

  async function request(url, options) {
    const response = await fetch(url, options);
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || `Request failed (${response.status})`);
    return data;
  }

  function render(items) {
    list.replaceChildren();
    if (!items.length) {
      const empty = document.createElement('p');
      empty.className = 'work-activity-empty';
      empty.textContent = 'No history yet. Add a note or update the ticket/task to start the timeline.';
      list.append(empty);
      return;
    }
    items.forEach((item) => {
      const entry = document.createElement('article'); entry.className = 'work-activity-entry';
      const meta = document.createElement('div'); meta.className = 'work-activity-meta';
      const actor = document.createElement('span'); actor.className = 'work-activity-actor'; actor.textContent = item.actor_name || 'Team member';
      const actorRole = document.createElement('span'); actorRole.className = 'work-activity-role'; actorRole.textContent = item.actor_role || 'staff';
      const role = document.createElement('span'); role.className = 'work-activity-kind'; role.textContent = item.kind === 'comment' ? 'Work note' : (item.kind || 'Update').replaceAll('_', ' ');
      const date = document.createElement('time'); date.textContent = item.created_at ? new Date(item.created_at).toLocaleString() : '';
      meta.append(actor, actorRole, role, date);
      const body = document.createElement('p'); body.className = 'work-activity-message'; body.textContent = item.message || '';
      entry.append(meta, body); list.append(entry);
    });
    list.scrollTop = list.scrollHeight;
  }

  async function load() {
    if (!activeWork) return;
    list.innerHTML = '<p class="work-activity-empty">Loading work history…</p>';
    try {
      const data = await request(`/api/work/${activeWork.type}/${encodeURIComponent(activeWork.id)}/activity?limit=100`);
      render(data.activities || []);
    } catch (loadError) {
      list.replaceChildren();
      const empty = document.createElement('p'); empty.className = 'work-activity-empty'; empty.textContent = loadError.message;
      list.append(empty);
    }
  }

  async function loadAttachments() {
    if (!activeWork) return;
    attachmentList.replaceChildren();
    attachmentError.textContent = '';
    try {
      const data = await request(`/api/work/${activeWork.type}/${encodeURIComponent(activeWork.id)}/attachments`);
      const items = data.attachments || [];
      if (!items.length) {
        const empty = document.createElement('p'); empty.className = 'work-attachment-empty'; empty.textContent = 'No files attached yet.';
        attachmentList.append(empty);
        return;
      }
      items.forEach((item) => {
        const link = document.createElement('a');
        link.className = 'work-attachment-link';
        link.href = `/api/work/${activeWork.type}/${encodeURIComponent(activeWork.id)}/attachments/${encodeURIComponent(item.id)}`;
        link.setAttribute('download', item.original_name || 'attachment');
        link.title = `${item.original_name || 'Attachment'} · ${formatFileSize(item.size)}`;
        link.textContent = `${item.original_name || 'Attachment'} · ${formatFileSize(item.size)}`;
        attachmentList.append(link);
      });
    } catch (loadError) {
      attachmentError.textContent = loadError.message;
    }
  }

  function formatFileSize(value) {
    const size = Number(value) || 0;
    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  }

  function close() {
    dialog.classList.remove('open'); document.body.style.overflow = '';
    if (previousFocus?.isConnected) previousFocus.focus();
    activeWork = null;
  }

  document.addEventListener('click', (event) => {
    const button = event.target.closest('[data-work-activity][data-work-id]');
    if (!button) return;
    const type = button.dataset.workActivity;
    if (type !== 'ticket' && type !== 'task') return;
    activeWork = { type, id: button.dataset.workId };
    previousFocus = button;
    title.textContent = button.dataset.workTitle || `${type} ${button.dataset.workId}`;
    form.reset(); error.textContent = '';
    attachmentInput.value = ''; attachmentError.textContent = '';
    dialog.classList.add('open'); document.body.style.overflow = 'hidden';
    dialog.querySelector('.work-activity-close').focus();
    load(); loadAttachments();
  });

  dialog.addEventListener('click', (event) => {
    if (event.target === dialog || event.target.closest('.work-activity-close')) close();
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && dialog.classList.contains('open')) close();
  });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!activeWork) return;
    const text = message.value.trim();
    if (!text) { error.textContent = 'Write a note first.'; return; }
    submit.disabled = true; submit.textContent = 'Saving…'; error.textContent = '';
    try {
      await request(`/api/work/${activeWork.type}/${encodeURIComponent(activeWork.id)}/comments`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ message: text })
      });
      form.reset(); await load();
    } catch (saveError) { error.textContent = saveError.message; }
    finally { submit.disabled = false; submit.textContent = 'Add note'; }
  });

  attachmentButton.addEventListener('click', () => attachmentInput.click());
  attachmentInput.addEventListener('change', async () => {
    const file = attachmentInput.files && attachmentInput.files[0];
    if (!file || !activeWork) return;
    attachmentButton.disabled = true; attachmentButton.textContent = 'Uploading…'; attachmentError.textContent = '';
    const body = new FormData(); body.append('file', file);
    try {
      await request(`/api/work/${activeWork.type}/${encodeURIComponent(activeWork.id)}/attachments`, { method: 'POST', body });
      attachmentInput.value = '';
      await Promise.all([loadAttachments(), load()]);
    } catch (uploadError) {
      attachmentError.textContent = uploadError.message;
    } finally {
      attachmentButton.disabled = false; attachmentButton.textContent = 'Add file';
    }
  });
})();
