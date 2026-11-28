(() => {
  'use strict';
  const storageKey = 'ost-theme';
  const root = document.documentElement;
  const toggles = [...document.querySelectorAll('[data-theme-toggle]')];
  const isDark = () => root.classList.contains('dark');

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
  }).catch(() => {});
})();
