(() => {
  'use strict';
  const storageKey = 'ost-theme';
  const root = document.documentElement;
  const toggle = document.getElementById('themeToggle');
  const isDark = () => root.classList.contains('dark');

  function updateToggle() {
    if (!toggle) return;
    const dark = isDark();
    toggle.textContent = dark ? '☀️ Light mode' : '🌙 Dark mode';
    toggle.setAttribute('aria-pressed', String(dark));
    toggle.setAttribute('aria-label', dark ? 'Switch to light mode' : 'Switch to dark mode');
  }

  updateToggle();
  toggle?.addEventListener('click', () => {
    root.classList.toggle('dark');
    try { localStorage.setItem(storageKey, isDark() ? 'dark' : 'light'); } catch (_) { /* Storage may be disabled. */ }
    updateToggle();
  });
})();
