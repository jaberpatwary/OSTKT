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
})();
