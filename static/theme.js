(() => {
  'use strict';
  const storageKey = 'ost-theme';
  const root = document.documentElement;
  const toggles = [...document.querySelectorAll('[data-theme-toggle]')];
  const isDark = () => root.classList.contains('dark');

  const nativeFetch = window.fetch.bind(window);
  window.fetch = (resource, options = {}) => {
    const requestURL = new URL(resource instanceof Request ? resource.url : resource, location.href);
    const method = String(options.method || (resource instanceof Request ? resource.method : 'GET')).toUpperCase();
    if (requestURL.origin === location.origin && !['GET', 'HEAD', 'OPTIONS'].includes(method)) {
      const headers = new Headers(resource instanceof Request ? resource.headers : undefined);
      new Headers(options.headers || {}).forEach((value, key) => headers.set(key, value));
      const cookie = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith('ost_csrf='));
      if (cookie) headers.set('X-CSRF-Token', decodeURIComponent(cookie.slice('ost_csrf='.length)));
      options = { ...options, headers };
    }
    return nativeFetch(resource, options);
  };

  function updateToggles() {
    const dark = isDark();
    toggles.forEach((toggle) => {
      toggle.textContent = dark ? '☀️ Light mode' : '🌙 Dark mode';
      toggle.setAttribute('aria-pressed', String(dark));
      toggle.setAttribute('aria-label', dark ? 'Switch to light mode' : 'Switch to dark mode');
    });
  }

  updateToggles();
  toggles.forEach((toggle) => toggle.addEventListener('click', () => {
    root.classList.toggle('dark');
    try { localStorage.setItem(storageKey, isDark() ? 'dark' : 'light'); } catch (_) { /* Storage may be disabled. */ }
    updateToggles();
  }));

  document.querySelectorAll('[data-roles]').forEach((link) => { link.style.display = 'none'; });
  fetch('/api/auth/me').then((response) => response.ok ? response.json() : null).then((data) => {
    if (!data?.user?.role) return;
    document.querySelectorAll('[data-roles]').forEach((link) => {
      const roles = (link.dataset.roles || '').split(',').map((role) => role.trim());
      link.style.display = roles.includes(data.user.role) ? '' : 'none';
    });
    const button = document.createElement('button');
    button.type = 'button'; button.textContent = 'Sign out';
    button.title = 'Signed in as ' + (data.user.display_name || data.user.username);
    button.className = 'rounded-xl border border-slate-300 bg-white/80 px-3 py-2 text-xs font-semibold text-slate-700 hover:bg-slate-100';
    button.addEventListener('click', async () => {
      button.disabled = true;
      try { await fetch('/api/auth/logout', { method: 'POST' }); } catch (_) {}
      location.replace('/login');
    });
    const nav = document.querySelector('main header nav, header nav');
    if (nav) {
      const workspaceLink = document.createElement('a');
      workspaceLink.href = '/workspace'; workspaceLink.textContent = 'Operations workspace';
      workspaceLink.className = 'rounded-xl border border-teal-300 bg-teal-50/80 px-3 py-2 text-xs font-semibold text-teal-800 hover:bg-teal-100';
      const securityLink = document.createElement('a');
      securityLink.href = '/account/security'; securityLink.textContent = 'Account security';
      securityLink.className = 'rounded-xl border border-slate-300 bg-white/80 px-3 py-2 text-xs font-semibold text-slate-700 hover:bg-slate-100';
      nav.append(workspaceLink, securityLink, button);
    }
    else {
      button.style.position = 'fixed'; button.style.top = '12px'; button.style.right = '12px'; button.style.zIndex = '10000';
      document.body.append(button);
    }
  }).catch(() => {});
})();
