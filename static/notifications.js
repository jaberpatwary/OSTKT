(() => {
  'use strict';

  if (document.getElementById('deskNotifications')) return;

  const escapeHTML = (value) => String(value ?? '').replace(/[&<>"']/g, (ch) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[ch]));

  const style = document.createElement('style');
  style.textContent = `
    #deskNotifications{position:fixed;left:20px;bottom:20px;z-index:10020;font-family:Manrope,system-ui,sans-serif;color:#e8eefc}
    #deskNotificationButton{position:relative;width:54px;height:54px;border:1px solid #53648b;border-radius:18px;background:#17243b;color:#f8fafc;font-size:22px;box-shadow:0 12px 36px #050b18aa;cursor:pointer}
    #deskNotificationBadge{position:absolute;top:-6px;right:-6px;min-width:21px;height:21px;padding:0 5px;border:2px solid #111a2b;border-radius:999px;background:#ef5574;color:white;font:800 11px/17px Manrope,system-ui,sans-serif;text-align:center}
    #deskNotificationPanel{position:absolute;left:0;bottom:66px;width:min(420px,calc(100vw - 32px));max-height:min(70vh,620px);overflow:hidden;border:1px solid #354462;border-radius:20px;background:#101a2c;box-shadow:0 22px 65px #020617bb}
    #deskNotificationPanel[hidden]{display:none}
    .desk-notice-head{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:16px;border-bottom:1px solid #293750;background:linear-gradient(115deg,#1b2944,#153a43)}
    .desk-notice-list{max-height:min(54vh,470px);overflow:auto;padding:8px}
    .desk-notice-row{display:block;margin:4px 0;padding:12px;border:1px solid #293750;border-radius:13px;background:#151f33;color:inherit;text-decoration:none}
    .desk-notice-row:hover{border-color:#5572aa;background:#1a2942}
    .desk-notice-row.unread{border-left:3px solid #43d4ba}
    .desk-notice-title{font-size:13px;font-weight:800;line-height:1.45;color:#f1f5ff}
    .desk-notice-message{margin-top:4px;font-size:12px;line-height:1.45;color:#afbbd1;white-space:pre-wrap;overflow-wrap:anywhere}
    .desk-notice-meta{display:flex;justify-content:space-between;gap:8px;margin-top:8px;font-size:10px;color:#8493ad}
    .desk-notice-action{border:0;background:transparent;color:#7ce5d0;font-size:11px;font-weight:800;cursor:pointer}
    .desk-notice-empty{padding:30px 16px;text-align:center;color:#9aa8bf;font-size:13px}
    @media(max-width:480px){#deskNotifications{left:14px;bottom:14px}#deskNotificationPanel{left:-2px}}
  `;
  document.head.appendChild(style);

  const root = document.createElement('div');
  root.id = 'deskNotifications';
  root.innerHTML = `
    <section id="deskNotificationPanel" hidden aria-label="Notifications">
      <header class="desk-notice-head">
        <div><strong style="display:block;font-size:14px">Work alerts</strong><span id="deskNotificationSummary" style="font-size:11px;color:#aab8ce">Loading…</span></div>
        <button id="deskMarkAllRead" class="desk-notice-action" type="button">Mark all read</button>
      </header>
      <div id="deskNotificationList" class="desk-notice-list" aria-live="polite"></div>
    </section>
    <button id="deskNotificationButton" type="button" aria-label="Open notifications" aria-expanded="false" title="Work alerts">🔔<span id="deskNotificationBadge" hidden>0</span></button>
  `;
  document.body.appendChild(root);

  const button = document.getElementById('deskNotificationButton');
  const panel = document.getElementById('deskNotificationPanel');
  const list = document.getElementById('deskNotificationList');
  const badge = document.getElementById('deskNotificationBadge');
  const summary = document.getElementById('deskNotificationSummary');
  const markAll = document.getElementById('deskMarkAllRead');

  async function request(url, options) {
    const response = await fetch(url, options);
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || `Request failed (${response.status})`);
    return data;
  }

  function render(items, unreadCount) {
    badge.hidden = unreadCount < 1;
    badge.textContent = unreadCount > 99 ? '99+' : String(unreadCount);
    summary.textContent = unreadCount ? `${unreadCount} unread alert${unreadCount === 1 ? '' : 's'}` : 'You are all caught up';
    markAll.hidden = unreadCount < 1;
    if (!items.length) {
      list.innerHTML = '<div class="desk-notice-empty">No work alerts right now.</div>';
      return;
    }
    list.innerHTML = items.map((item) => {
      const href = new URL(item.link || '/', location.origin);
      const safeHref = href.origin === location.origin ? href.pathname + href.search : '/';
      const timestamp = item.created_at ? new Date(item.created_at).toLocaleString() : '';
      const severity = item.severity === 'urgent' ? ' · OVERDUE' : '';
      return `<a href="${escapeHTML(safeHref)}" class="desk-notice-row ${item.read_at ? '' : 'unread'}" data-notification-id="${Number(item.id)}" data-notification-link="${escapeHTML(safeHref)}">
        <div class="desk-notice-title">${escapeHTML(item.title)}${severity}</div>
        <div class="desk-notice-message">${escapeHTML(item.message)}</div>
        <div class="desk-notice-meta"><span>${escapeHTML(timestamp)}</span><span>${item.read_at ? 'Read' : 'Open'}</span></div>
      </a>`;
    }).join('');
  }

  async function load() {
    try {
      const data = await request('/api/notifications?limit=30');
      render(data.notifications || [], Number(data.unread_count) || 0);
    } catch (error) {
      summary.textContent = 'Alerts are temporarily unavailable';
      list.innerHTML = `<div class="desk-notice-empty">${escapeHTML(error.message)}</div>`;
    }
  }

  button.addEventListener('click', async () => {
    const open = panel.hidden;
    panel.hidden = !open;
    button.setAttribute('aria-expanded', String(open));
    if (open) await load();
  });

  document.addEventListener('click', (event) => {
    if (!panel.hidden && !root.contains(event.target)) {
      panel.hidden = true;
      button.setAttribute('aria-expanded', 'false');
    }
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && !panel.hidden) {
      panel.hidden = true;
      button.setAttribute('aria-expanded', 'false');
      button.focus();
    }
  });

  list.addEventListener('click', async (event) => {
    const link = event.target.closest('[data-notification-id]');
    if (!link) return;
    event.preventDefault();
    try {
      await request(`/api/notifications/${encodeURIComponent(link.dataset.notificationId)}/read`, { method: 'POST' });
    } catch (_) { /* The destination is still useful when a read update fails. */ }
    location.assign(link.dataset.notificationLink || '/');
  });

  markAll.addEventListener('click', async () => {
    try { await request('/api/notifications/read-all', { method: 'POST' }); await load(); }
    catch (error) { summary.textContent = error.message; }
  });

  load();
  window.setInterval(load, 120000);
  window.addEventListener('focus', load);
})();
