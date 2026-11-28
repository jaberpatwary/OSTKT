(() => {
  'use strict';

  const taskMode = location.pathname.replace(/\/+$/, '') === '/admin/tasks';
  const kind = taskMode ? 'task' : 'ticket';
  const ids = taskMode
    ? { tbody: 'taskRows', table: 'taskRows', search: 'taskSearch', status: 'taskStatusFilter', day: 'taskDayFilter', refresh: 'taskAdminRefresh', error: 'taskAdminError', paging: 'Task pages' }
    : { tbody: 'ticketRows', table: 'ticketRows', search: 'searchInput', status: 'statusFilter', day: 'ticketDayFilter', refresh: 'refreshBtn', error: 'errorBox', paging: 'Ticket pages' };
  const $ = (id) => document.getElementById(id);
  const bodyRows = $(ids.tbody);
  if (!bodyRows) return;

  const assignees = ['Rupok Kumar Roy', 'Towhedul Islam', 'Md. Imam Hossan Chowdhury', 'Muhammad Fokhrul Hasan', 'Md. Tanvir Ahamed Sarkar', 'Md. Fahad Hossen', 'Mabubul Alam Patwary', 'IT Team'];
  const statusOptions = [['pending', 'pending'], ['in_progress', 'inProgress'], ['done', 'done']];
  const language = {
    en: {
      searchAll:'Search all tickets and tasks', list:'List', kanban:'Kanban', selectPage:'Select page', clearPage:'Clear page', columns:'Columns',
      contrast:'Contrast', language:'বাংলা', savedViews:'Saved views', chooseView:'Choose saved view…', nameView:'Name this view',
      saveView:'Save current view', remove:'Remove', selected:'selected', statusPrompt:'Set status…', applyStatus:'Apply status',
      assignPrompt:'Assign to…', applyAssignee:'Apply assignee', clearSelection:'Clear selection', pending:'Pending',
      inProgress:'In progress', done:'Done', searchTitle:'Search all work', searchSubtitle:'Search tickets and tasks across all dates.',
      searchPlaceholder:'Employee, issue, task, or ID…', searchInitial:'Search across tickets and tasks, including previous dates.',
      searchMinimum:'Type at least two characters to search.', searching:'Searching tickets and tasks…',
      searchEmpty:'No matching tickets or tasks. Try an employee name, issue, task, or ID.', details:'Work details',
      detailsSubtitle:'Current values and ownership', openTimeline:'Open activity timeline', noPending:'No pending items in this view.',
      noProgress:'No in progress items in this view.', noDone:'No done items in this view.', priority:'Priority',
      assignee:'Assignee', givenBy:'Given by', due:'Due', sla:'SLA', taskDetails:'Task details', employee:'Employee',
      employeeId:'Employee ID', department:'Department', problem:'Problem', assetTag:'Asset tag', createdBy:'Created by',
      created:'Created', updated:'Updated', paused:'Paused', overdue:'Overdue', dueSoon:'Due soon', resolved:'Resolved',
      noDate:'No due date', updatePrompt:'Apply this change to', items:'items?', updatedMessage:'Updated',
      itemsSelected:'items selected. Bulk changes apply only after you choose an action.',
      updatedBeforeFailure:'updated before the next item failed.', searchFailed:'Search failed. Try again.'
    },
    bn: {
      searchAll:'সব টিকিট ও টাস্ক খুঁজুন', list:'তালিকা', kanban:'কানবান', selectPage:'পৃষ্ঠা নির্বাচন', clearPage:'পৃষ্ঠা বাতিল', columns:'কলাম',
      contrast:'কনট্রাস্ট', language:'English', savedViews:'সংরক্ষিত ভিউ', chooseView:'ভিউ বেছে নিন…', nameView:'ভিউয়ের নাম',
      saveView:'ভিউ সংরক্ষণ', remove:'মুছুন', selected:'নির্বাচিত', statusPrompt:'স্ট্যাটাস দিন…', applyStatus:'স্ট্যাটাস প্রয়োগ',
      assignPrompt:'দায়িত্ব দিন…', applyAssignee:'দায়িত্ব প্রয়োগ', clearSelection:'নির্বাচন বাতিল', pending:'পেন্ডিং',
      inProgress:'চলমান', done:'সম্পন্ন', searchTitle:'সব কাজ খুঁজুন', searchSubtitle:'আগের তারিখসহ সব টিকিট ও টাস্ক খুঁজুন।',
      searchPlaceholder:'কর্মীর নাম, সমস্যা, টাস্ক বা ID…', searchInitial:'টিকিট ও টাস্ক খুঁজতে লিখুন। আগের তারিখও খোঁজা হবে।',
      searchMinimum:'খুঁজতে অন্তত ২টি অক্ষর লিখুন।', searching:'টিকিট ও টাস্ক খোঁজা হচ্ছে…',
      searchEmpty:'কোনো মিল পাওয়া যায়নি। কর্মীর নাম, সমস্যা, টাস্ক বা ID দিয়ে খুঁজুন।', details:'কাজের বিবরণ',
      detailsSubtitle:'বর্তমান তথ্য ও দায়িত্ব', openTimeline:'কাজের ইতিহাস খুলুন', noPending:'এই ভিউতে পেন্ডিং কাজ নেই।',
      noProgress:'এই ভিউতে চলমান কাজ নেই।', noDone:'এই ভিউতে সম্পন্ন কাজ নেই।', priority:'অগ্রাধিকার',
      assignee:'দায়িত্বপ্রাপ্ত', givenBy:'প্রদানকারী', due:'শেষ তারিখ', sla:'SLA', taskDetails:'কাজের বিবরণ',
      employee:'কর্মী', employeeId:'কর্মী ID', department:'বিভাগ', problem:'সমস্যা', assetTag:'অ্যাসেট ট্যাগ',
      createdBy:'তৈরি করেছেন', created:'তৈরি', updated:'আপডেট', paused:'স্থগিত', overdue:'সময় পেরিয়েছে',
      dueSoon:'শিগগির শেষ', resolved:'সমাধান হয়েছে', noDate:'তারিখ নেই', updatePrompt:'নির্বাচিত',
      items:'টি আইটেমে পরিবর্তন করবেন?', updatedMessage:'আপডেট হয়েছে', itemsSelected:'টি আইটেম নির্বাচিত। পরিবর্তন করতে action বেছে নিন।',
      updatedBeforeFailure:'টি আপডেট হয়েছে, এরপরের আইটেমে সমস্যা হয়েছে।', searchFailed:'খোঁজা যায়নি। আবার চেষ্টা করুন।'
    }
  };
  Object.assign(language.en, {
    detailsAction:'Details', history:'History', save:'Save', markDone:'Done',
    ticketAdmin:'Admin Panel', taskAdmin:'Task Admin Panel', ticketList:'Ticket list', taskList:'Manage tasks',
    totalTickets:'Total tickets', totalTasks:'All tasks', pendingCount:'Pending', doneCount:'Completed',
    progressCount:'In progress', overdueCount:'SLA overdue', dueSoonCount:'Due in 4 hours', allStatuses:'All statuses'
  });
  Object.assign(language.bn, {
    detailsAction:'বিবরণ', history:'ইতিহাস', save:'সংরক্ষণ', markDone:'সম্পন্ন',
    ticketAdmin:'টিকিট অ্যাডমিন প্যানেল', taskAdmin:'টাস্ক অ্যাডমিন প্যানেল', ticketList:'টিকিট তালিকা', taskList:'টাস্ক পরিচালনা',
    totalTickets:'মোট টিকিট', totalTasks:'মোট টাস্ক', pendingCount:'পেন্ডিং', doneCount:'সম্পন্ন',
    progressCount:'চলমান', overdueCount:'SLA সময় পেরিয়েছে', dueSoonCount:'৪ ঘণ্টার মধ্যে শেষ', allStatuses:'সব স্ট্যাটাস'
  });
  let locale = 'en';
  try { locale = localStorage.getItem('ost-admin-locale') === 'bn' ? 'bn' : 'en'; } catch (_) {}
  const t = (key) => language[locale][key] || language.en[key] || key;
  const records = new Map();
  const selected = new Set();
  let activeView = 'list';
  let lastFocus = null;
  let searchTimer = 0;

  function node(tag, className, text) {
    const item = document.createElement(tag);
    if (className) item.className = className;
    if (text != null) item.textContent = String(text);
    return item;
  }
  function button(text, className, title) {
    const item = node('button', className, text);
    item.type = 'button';
    if (title) item.title = title;
    return item;
  }
  function formatDate(value) {
    if (!value) return '—';
    const date = /^\d{4}-\d{2}-\d{2}$/.test(String(value)) ? new Date(value + 'T00:00:00+06:00') : new Date(value);
    return Number.isNaN(date.getTime()) ? String(value) : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeZone: 'Asia/Dhaka' }).format(date);
  }
  function localDay(value) {
    if (!value) return '';
    return new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Dhaka' }).format(new Date(value));
  }
  async function api(url, options) {
    const response = await fetch(url, { cache: 'no-store', ...(options || {}) });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || 'Request failed (' + response.status + ')');
    return data;
  }

  const queueSection = bodyRows.closest('section');
  const table = bodyRows.closest('table');
  const tableViewport = table && table.parentElement;
  if (!queueSection || !tableViewport) return;

  const tools = node('div', 'workspace-tools');
  tools.setAttribute('aria-label', 'Workspace tools');
  const searchAll = button('', 'workspace-search-all', 'Search tickets and tasks (Ctrl+K)');
  searchAll.innerHTML = '<span></span><kbd>Ctrl K</kbd>';
  searchAll.querySelector('span').textContent = t('searchAll');
  searchAll.setAttribute('aria-haspopup', 'dialog');
  searchAll.setAttribute('aria-keyshortcuts', 'Control+K Meta+K');
  tools.append(searchAll);

  const viewToggle = node('div', 'workspace-view-toggle');
  viewToggle.setAttribute('role', 'group');
  viewToggle.setAttribute('aria-label', 'Choose list or board view');
  const listViewButton = button(t('list'), '', 'Show table view');
  const boardViewButton = button(t('kanban'), '', 'Show status board');
  listViewButton.setAttribute('aria-pressed', 'true');
  boardViewButton.setAttribute('aria-pressed', 'false');
  viewToggle.append(listViewButton, boardViewButton);
  tools.append(viewToggle);
  const selectPageButton = button(t('selectPage'), '', 'Select all items on this page');
  selectPageButton.setAttribute('aria-pressed', 'false');
  tools.append(selectPageButton);

  const columnsMenu = node('div', 'workspace-columns-menu');
  const columnsButton = button(t('columns'), '', 'Choose which table columns are visible');
  columnsButton.setAttribute('aria-expanded', 'false');
  columnsButton.setAttribute('aria-haspopup', 'true');
  const columnsList = node('div', 'workspace-columns-list');
  columnsList.setAttribute('role', 'group');
  columnsList.setAttribute('aria-label', 'Visible table columns');
  columnsMenu.append(columnsButton, columnsList);
  tools.append(columnsMenu);

  const accessMenu = node('div', 'workspace-accessibility');
  accessMenu.setAttribute('role', 'group');
  accessMenu.setAttribute('aria-label', 'Accessibility controls');
  const smallerText = button('A−', '', 'Decrease text size');
  const largerText = button('A+', '', 'Increase text size');
  const contrastButton = button(t('contrast'), '', 'Toggle high contrast');
  const languageButton = button(locale === 'en' ? 'বাংলা' : 'English', '', 'Switch between English and Bangla workspace controls');
  languageButton.setAttribute('aria-label', 'Switch workspace language');
  accessMenu.append(smallerText, largerText, contrastButton, languageButton);
  tools.append(accessMenu);

  const savedBar = node('div', 'workspace-saved-views');
  const savedLabel = node('span', 'workspace-saved-views-label', t('savedViews'));
  savedBar.append(savedLabel);
  const savedSelect = node('select');
  savedSelect.setAttribute('aria-label', 'Load a saved view');
  savedSelect.append(new Option(t('chooseView'), ''));
  const savedName = node('input');
  savedName.type = 'text';
  savedName.maxLength = 40;
  savedName.placeholder = t('nameView');
  savedName.setAttribute('aria-label', 'Name this saved view');
  const saveViewButton = button(t('saveView'));
  const deleteViewButton = button(t('remove'));
  savedBar.append(savedSelect, savedName, saveViewButton, deleteViewButton);

  const bulkbar = node('div', 'workspace-bulkbar');
  bulkbar.setAttribute('aria-live', 'polite');
  const selectedCount = node('strong', '', '0 ' + t('selected'));
  const bulkStatus = node('select');
  bulkStatus.setAttribute('aria-label', 'Set status for selected items');
  bulkStatus.append(new Option(t('statusPrompt'), ''));
  statusOptions.forEach(([value, labelKey]) => bulkStatus.append(new Option(t(labelKey), value)));
  const bulkStatusButton = button(t('applyStatus'), 'workspace-apply');
  const bulkOwner = node('select');
  bulkOwner.setAttribute('aria-label', 'Assign selected items');
  bulkOwner.append(new Option(t('assignPrompt'), ''));
  if (!taskMode) bulkOwner.append(new Option('Unassigned', '__unassigned__'));
  assignees.forEach((name) => bulkOwner.append(new Option(name, name)));
  const bulkOwnerButton = button(t('applyAssignee'));
  const clearSelectionButton = button(t('clearSelection'), 'workspace-clear');
  const bulkMessage = node('span', 'workspace-live');
  bulkMessage.id = 'workspaceBulkAnnouncement';
  bulkbar.append(selectedCount, bulkStatus, bulkStatusButton, bulkOwner, bulkOwnerButton, clearSelectionButton);

  const board = node('div', 'workspace-view-root workspace-board');
  board.id = 'workspaceKanban';
  board.setAttribute('aria-label', kind + ' status board');
  const statuses = [
    { id: 'pending', title: 'pending' },
    { id: 'in_progress', title: 'inProgress' },
    { id: 'done', title: 'done' }
  ];
  const laneLists = new Map();
  statuses.forEach((status) => {
    const lane = node('section', 'workspace-lane');
    lane.setAttribute('aria-labelledby', 'workspaceLane-' + status.id);
    const header = node('div', 'workspace-lane-head');
    const heading = node('h3', '', t(status.title));
    heading.id = 'workspaceLane-' + status.id;
    const count = node('span', 'workspace-lane-count', '0');
    const list = node('div', 'workspace-lane-list');
    header.append(heading, count); lane.append(header, list); board.append(lane);
    laneLists.set(status.id, { list, count });
  });

  tableViewport.insertAdjacentElement('afterend', board);
  const pager = [...queueSection.querySelectorAll('[role="navigation"]')].find((item) => item.getAttribute('aria-label') === ids.paging);
  if (pager) pager.insertAdjacentElement('afterend', bulkMessage);
  const queueHeader = tableViewport.previousElementSibling;
  if (queueHeader) queueHeader.insertAdjacentElement('afterend', tools);
  tools.insertAdjacentElement('afterend', savedBar);
  savedBar.insertAdjacentElement('afterend', bulkbar);

  function storageGet(key, fallback) {
    try { return JSON.parse(localStorage.getItem(key) || 'null') ?? fallback; } catch (_) { return fallback; }
  }
  function storageSet(key, value) {
    try { localStorage.setItem(key, JSON.stringify(value)); } catch (_) { /* Private browsing may disable storage. */ }
  }
  function viewStorageKey() { return 'ost-saved-views-' + kind; }
  function refreshSavedViews() {
    const views = storageGet(viewStorageKey(), []);
    savedSelect.replaceChildren(new Option(t('chooseView'), ''));
    views.forEach((view) => savedSelect.append(new Option(view.name, view.id)));
    deleteViewButton.disabled = !savedSelect.value;
  }
  function currentView() {
    return {
      query: $(ids.search).value,
      status: $(ids.status).value,
      day: $(ids.day).value
    };
  }
  function triggerFilters(previous) {
    const changedDay = $(ids.day).value !== previous.day;
    const changedStatus = $(ids.status).value !== previous.status;
    if (changedDay) $(ids.day).dispatchEvent(new Event('change', { bubbles: true }));
    else if (changedStatus) $(ids.status).dispatchEvent(new Event('change', { bubbles: true }));
    else $(ids.search).dispatchEvent(new Event('input', { bubbles: true }));
  }
  saveViewButton.addEventListener('click', () => {
    const name = savedName.value.trim();
    if (!name) { savedName.focus(); savedName.setCustomValidity('Enter a name for this view.'); savedName.reportValidity(); return; }
    savedName.setCustomValidity('');
    const views = storageGet(viewStorageKey(), []);
    const view = { id: String(Date.now()), name, ...currentView() };
    views.unshift(view);
    storageSet(viewStorageKey(), views.slice(0, 12));
    savedName.value = '';
    refreshSavedViews();
    savedSelect.value = view.id;
    savedSelect.dispatchEvent(new Event('change'));
  });
  savedName.addEventListener('input', () => savedName.setCustomValidity(''));
  savedSelect.addEventListener('change', () => {
    const view = storageGet(viewStorageKey(), []).find((item) => item.id === savedSelect.value);
    deleteViewButton.disabled = !view;
    if (!view) return;
    const previous = currentView();
    $(ids.search).value = view.query || '';
    $(ids.status).value = view.status || '';
    $(ids.day).value = view.day || new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Dhaka' }).format(new Date());
    triggerFilters(previous);
  });
  deleteViewButton.addEventListener('click', () => {
    if (!savedSelect.value) return;
    storageSet(viewStorageKey(), storageGet(viewStorageKey(), []).filter((item) => item.id !== savedSelect.value));
    refreshSavedViews();
  });
  refreshSavedViews();

  const columns = [...table.querySelectorAll('thead th')];
  const columnKey = 'ost-visible-columns-' + kind;
  const defaultColumns = columns.map((_, index) => index);
  const visibleColumns = new Set(storageGet(columnKey, defaultColumns));
  columns.forEach((column, index) => {
    const label = column.textContent.trim() || ('Column ' + (index + 1));
    const wrapper = node('label');
    const check = document.createElement('input');
    check.type = 'checkbox'; check.checked = visibleColumns.has(index);
    check.addEventListener('change', () => {
      if (check.checked) visibleColumns.add(index); else visibleColumns.delete(index);
      if (visibleColumns.size === 0) { check.checked = true; visibleColumns.add(index); }
      storageSet(columnKey, [...visibleColumns]);
      applyColumns();
    });
    wrapper.append(check, document.createTextNode(label));
    columnsList.append(wrapper);
  });
  function applyColumns() {
    [...table.querySelectorAll('tr')].forEach((row) => {
      [...row.children].forEach((cell, index) => { cell.hidden = !visibleColumns.has(index); });
    });
    columns.forEach((column, index) => { column.hidden = !visibleColumns.has(index); });
    table.style.minWidth = Math.max(620, visibleColumns.size * 140) + 'px';
  }
  columnsButton.addEventListener('click', () => {
    const open = columnsMenu.classList.toggle('is-open');
    columnsButton.setAttribute('aria-expanded', String(open));
  });
  document.addEventListener('click', (event) => {
    if (!columnsMenu.contains(event.target)) {
      columnsMenu.classList.remove('is-open');
      columnsButton.setAttribute('aria-expanded', 'false');
    }
  });

  function syncSelection() {
    const count = selected.size;
    selectedCount.textContent = count + ' ' + t('selected');
    bulkbar.classList.toggle('is-visible', count > 0);
    bulkMessage.textContent = count ? count + ' ' + t('itemsSelected') : '';
    bodyRows.querySelectorAll('[data-work-select]').forEach((checkbox) => {
      checkbox.checked = selected.has(checkbox.dataset.workSelect);
    });
    board.querySelectorAll('[data-work-select]').forEach((checkbox) => {
      checkbox.checked = selected.has(checkbox.dataset.workSelect);
    });
    const pageIds = [...bodyRows.querySelectorAll('[data-work-select]')].map((checkbox) => checkbox.dataset.workSelect);
    const allSelected = pageIds.length > 0 && pageIds.every((id) => selected.has(id));
    selectPageButton.textContent = allSelected ? t('clearPage') : t('selectPage');
    selectPageButton.setAttribute('aria-pressed', String(allSelected));
  }
  selectPageButton.addEventListener('click', () => {
    const pageIds = [...bodyRows.querySelectorAll('[data-work-select]')].map((checkbox) => checkbox.dataset.workSelect);
    const allSelected = pageIds.length > 0 && pageIds.every((id) => selected.has(id));
    pageIds.forEach((id) => { if (allSelected) selected.delete(id); else selected.add(id); });
    syncSelection();
  });
  [$(ids.search), $(ids.status), $(ids.day)].forEach((control) => control.addEventListener('change', () => {
    records.clear(); renderBoard([]);
    if (selected.size) { selected.clear(); syncSelection(); }
  }));
  $(ids.search).addEventListener('input', () => {
    records.clear(); renderBoard([]);
    if (selected.size) { selected.clear(); syncSelection(); }
  });
  function rowSelection(record, container) {
    const cell = container.children[0];
    if (!cell || cell.querySelector('[data-work-select]')) return;
    const checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    checkbox.className = 'workspace-select-row';
    checkbox.dataset.workSelect = String(record.id);
    checkbox.setAttribute('aria-label', 'Select ' + (taskMode ? (record.title || 'task') : (record.ticket_number || 'ticket')));
    checkbox.addEventListener('change', () => {
      if (checkbox.checked) selected.add(String(record.id)); else selected.delete(String(record.id));
      syncSelection();
    });
    cell.prepend(checkbox);
  }
  function detailButton(record) {
    const result = button(t('detailsAction'), 'workspace-row-details', 'Open item details');
    result.dataset.workDetails = String(record.id);
    result.dataset.workType = kind;
    return result;
  }
  function ensureRowActions(record, row) {
    const actionCell = row.lastElementChild;
    if (!actionCell || actionCell.querySelector('[data-work-details]')) return;
    actionCell.append(detailButton(record));
  }

  function statusText(status) {
    const entry = statusOptions.find((item) => item[0] === status);
    return entry ? t(entry[1]) : status || t('pending');
  }
  function isOverdue(record) {
    if (record.status === 'done') return false;
    if (taskMode && record.due_date) return new Date(record.due_date + 'T23:59:59') < new Date();
    if (!taskMode && !record.sla_paused && record.sla_due_at) return new Date(record.sla_due_at) < new Date();
    return false;
  }
  function cardFor(record) {
    const card = node('article', 'workspace-work-card');
    const overdue = isOverdue(record);
    const dueSoon = !overdue && !taskMode && record.sla_due_at && new Date(record.sla_due_at).getTime() < Date.now() + 4 * 60 * 60 * 1000 && record.status !== 'done' && !record.sla_paused;
    card.classList.toggle('is-overdue', overdue);
    card.classList.toggle('is-due-soon', Boolean(dueSoon));
    const title = taskMode
      ? (record.title || record.description || ('Task ' + record.id))
      : ((record.ticket_number || 'Ticket') + ' · ' + (record.employee?.Name || 'Employee'));
    const head = node('div', 'workspace-work-card-head');
    head.append(node('strong', 'workspace-work-title', title));
    const checkbox = document.createElement('input');
    checkbox.type = 'checkbox'; checkbox.className = 'workspace-select-row';
    checkbox.dataset.workSelect = String(record.id);
    checkbox.setAttribute('aria-label', 'Select ' + title);
    checkbox.checked = selected.has(String(record.id));
    checkbox.addEventListener('change', () => {
      if (checkbox.checked) selected.add(String(record.id)); else selected.delete(String(record.id));
      syncSelection();
    });
    head.append(checkbox); card.append(head);
    const description = taskMode ? (record.description || '') : (record.problem || '');
    if (description) card.append(node('p', 'workspace-work-meta', description));
    const meta = node('div', 'workspace-work-meta');
    meta.append(node('span', '', statusText(record.status)));
    meta.append(node('span', '', t('priority') + ': ' + (record.priority || 'medium')));
    meta.append(node('span', '', t('assignee') + ': ' + (record.assigned_to || (taskMode ? 'IT Team' : 'Unassigned'))));
    if (taskMode) {
      meta.append(node('span', '', t('givenBy') + ': ' + (record.created_by || 'Not recorded')));
      meta.append(node('span', '', t('due') + ': ' + (record.due_date || t('noDate'))));
    } else {
      meta.append(node('span', '', t('sla') + ': ' + (record.sla_paused ? t('paused') : overdue ? t('overdue') : dueSoon ? t('dueSoon') : formatDate(record.sla_due_at))));
    }
    card.append(meta);
    const actions = node('div', 'workspace-work-actions');
    const status = node('select');
    status.setAttribute('aria-label', 'Change status for ' + title);
    statusOptions.forEach(([value, labelKey]) => status.append(new Option(t(labelKey), value)));
    status.value = record.status || 'pending';
    status.addEventListener('change', async () => {
      status.disabled = true;
      try {
        await updateRecord(record, 'status', status.value);
        $(ids.refresh).click();
      } catch (error) { showError(error.message); status.disabled = false; status.value = record.status || 'pending'; }
    });
    const open = detailButton(record);
    actions.append(status, open); card.append(actions);
    return card;
  }
  function renderBoard(items) {
    statuses.forEach((status) => {
      const lane = laneLists.get(status.id);
      const list = items.filter((record) => (record.status || 'pending') === status.id);
      lane.count.textContent = String(list.length);
      lane.list.replaceChildren(...list.map(cardFor));
      if (!list.length) lane.list.append(node('p', 'workspace-work-meta', t(status.id === 'pending' ? 'noPending' : status.id === 'done' ? 'noDone' : 'noProgress')));
    });
  }
  function enhanceRows(items) {
    items.forEach((record) => records.set(String(record.id), record));
    const rows = [...bodyRows.children];
    rows.forEach((row, index) => {
      const record = items[index];
      if (!record) return;
      row.dataset.workspaceType = kind;
      row.dataset.workspaceId = String(record.id);
      rowSelection(record, row);
      ensureRowActions(record, row);
    });
    bodyRows.querySelectorAll('select').forEach((select) => {
      [...select.options].forEach((option) => {
        const entry = statusOptions.find((item) => item[0] === option.value);
        if (entry) option.textContent = t(entry[1]);
      });
    });
    if (taskMode) bodyRows.querySelectorAll('tr[data-workspace-id]').forEach((row) => {
      const record = records.get(row.dataset.workspaceId);
      if (!record) return;
      row.querySelectorAll('[data-workspace-meta="priority"]').forEach((item) => { item.textContent = t('priority') + ' · ' + (record.priority || 'medium'); });
      row.querySelectorAll('[data-workspace-meta="due"]').forEach((item) => {
        item.textContent = t('due') + ' · ' + (record.due_date || t('noDate')) + (isOverdue(record) ? ' · ' + t('overdue') : '');
      });
    });
    renderBoard(items);
    applyColumns();
    syncSelection();
  }
  window.addEventListener('workspace:data', (event) => {
    if (event.detail?.type !== kind) return;
    enhanceRows(Array.isArray(event.detail.items) ? event.detail.items : []);
  });

  function showError(message) {
    const error = $(ids.error);
    if (!error) return;
    error.textContent = message || 'The update could not be completed.';
    error.classList.remove('hidden');
  }
  async function updateRecord(record, field, value) {
    if (taskMode) {
      const patch = {};
      patch[field === 'assignee' ? 'assigned_to' : field] = value;
      return api('/api/tasks/' + encodeURIComponent(record.id), {
        method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(patch)
      });
    }
    if (field === 'status') {
      return api('/api/tickets/' + encodeURIComponent(record.id) + '/status', {
        method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ status: value })
      });
    }
    return api('/api/tickets/' + encodeURIComponent(record.id) + '/assignee', {
      method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ assignee: value })
    });
  }
  async function applyBulk(field) {
    const value = field === 'status' ? bulkStatus.value : bulkOwner.value;
    if (!value || !selected.size) return;
    const idsToUpdate = [...selected];
    if (!window.confirm(t('updatePrompt') + ' ' + idsToUpdate.length + ' ' + t('selected') + ' ' + (taskMode ? 'tasks' : 'tickets') + '?')) return;
    bulkStatusButton.disabled = true; bulkOwnerButton.disabled = true;
    let completed = 0;
    try {
      for (const id of idsToUpdate) {
        const record = records.get(id);
        if (!record) continue;
        const next = field === 'assignee' && value === '__unassigned__' ? '' : value;
        await updateRecord(record, field, next);
        completed++;
      }
      selected.clear(); syncSelection();
      bulkMessage.textContent = t('updatedMessage') + ' ' + completed + ' ' + t('items');
      $(ids.refresh).click();
    } catch (error) {
      bulkMessage.textContent = completed + ' ' + t('updatedBeforeFailure');
      showError(error.message);
    } finally {
      bulkStatusButton.disabled = false; bulkOwnerButton.disabled = false;
      bulkStatus.value = ''; bulkOwner.value = '';
    }
  }
  bulkStatusButton.addEventListener('click', () => applyBulk('status'));
  bulkOwnerButton.addEventListener('click', () => applyBulk('assignee'));
  clearSelectionButton.addEventListener('click', () => { selected.clear(); syncSelection(); });

  function setView(view) {
    activeView = view;
    tableViewport.classList.toggle('workspace-table-hidden', view !== 'list');
    board.classList.toggle('is-active', view === 'board');
    listViewButton.setAttribute('aria-pressed', String(view === 'list'));
    boardViewButton.setAttribute('aria-pressed', String(view === 'board'));
    storageSet('ost-view-' + kind, view);
    renderBoard([...records.values()]);
  }
  listViewButton.addEventListener('click', () => setView('list'));
  boardViewButton.addEventListener('click', () => setView('board'));

  const searchDialog = node('div', 'workspace-overlay');
  searchDialog.id = 'workspaceSearchDialog';
  searchDialog.setAttribute('role', 'dialog');
  searchDialog.setAttribute('aria-modal', 'true');
  searchDialog.setAttribute('aria-labelledby', 'workspaceSearchTitle');
  const searchPanel = node('section', 'workspace-panel');
  const searchHead = node('header', 'workspace-panel-head');
  const searchHeadingWrap = node('div');
  const searchTitle = node('h2', 'workspace-panel-title', t('searchTitle'));
  searchTitle.id = 'workspaceSearchTitle';
  const searchSubtitle = node('p', 'workspace-panel-subtitle', t('searchSubtitle'));
  searchHeadingWrap.append(searchTitle, searchSubtitle);
  const searchClose = button('×', 'workspace-close', 'Close search');
  searchClose.setAttribute('aria-label', 'Close search');
  searchHead.append(searchHeadingWrap, searchClose);
  const universalInput = node('input', 'workspace-search-input');
  universalInput.type = 'search'; universalInput.placeholder = t('searchPlaceholder');
  universalInput.setAttribute('aria-label', 'Search tickets and tasks');
  universalInput.setAttribute('aria-controls', 'workspaceSearchResults');
  universalInput.setAttribute('aria-expanded', 'true');
  const searchResults = node('div', 'workspace-search-results');
  searchResults.id = 'workspaceSearchResults';
  searchResults.setAttribute('aria-live', 'polite');
  searchPanel.append(searchHead, universalInput, searchResults);
  searchDialog.append(searchPanel);
  document.body.append(searchDialog);

  const detailDialog = node('div', 'workspace-overlay');
  detailDialog.id = 'workspaceDetailDialog';
  detailDialog.setAttribute('role', 'dialog');
  detailDialog.setAttribute('aria-modal', 'true');
  detailDialog.setAttribute('aria-labelledby', 'workspaceDetailTitle');
  const detailPanel = node('section', 'workspace-panel');
  const detailHead = node('header', 'workspace-panel-head');
  const detailHeadingWrap = node('div');
  const detailTitle = node('h2', 'workspace-panel-title', t('details'));
  detailTitle.id = 'workspaceDetailTitle';
  const detailSubtitle = node('p', 'workspace-panel-subtitle', t('detailsSubtitle'));
  detailHeadingWrap.append(detailTitle, detailSubtitle);
  const detailClose = button('×', 'workspace-close', 'Close details');
  detailClose.setAttribute('aria-label', 'Close details');
  detailHead.append(detailHeadingWrap, detailClose);
  const detailGrid = node('div', 'workspace-details-grid');
  const detailActions = node('div', 'workspace-detail-actions');
  detailPanel.append(detailHead, detailGrid, detailActions);
  detailDialog.append(detailPanel);
  document.body.append(detailDialog);

  function detailItem(label, value, wide) {
    const item = node('div', 'workspace-detail' + (wide ? ' workspace-detail-wide' : ''));
    item.append(node('span', 'workspace-detail-label', label), node('span', 'workspace-detail-value', value || '—'));
    return item;
  }
  function openDetails(record, trigger) {
    lastFocus = trigger || document.activeElement;
    detailGrid.replaceChildren();
    detailActions.replaceChildren();
    if (taskMode) {
      detailTitle.textContent = record.title || ('Task ' + record.id);
      detailSubtitle.textContent = 'Task ' + String(record.id).padStart(6, '0') + ' · ' + statusText(record.status);
      detailGrid.append(
        detailItem(t('taskDetails'), record.description || record.title, true),
        detailItem(t('givenBy'), record.created_by),
        detailItem(t('assignee'), record.assigned_to || 'IT Team'),
        detailItem(t('priority'), record.priority),
        detailItem(t('due'), formatDate(record.due_date)),
        detailItem(t('created'), formatDate(record.created_at)),
        detailItem(t('updated'), formatDate(record.updated_at))
      );
    } else {
      const employee = record.employee || {};
      detailTitle.textContent = record.ticket_number || 'Ticket details';
      detailSubtitle.textContent = statusText(record.status) + ' · ' + (record.priority || 'medium') + ' priority';
      detailGrid.append(
        detailItem(t('employee'), employee.Name || 'Unknown'),
        detailItem(t('employeeId'), employee.EmployeeID),
        detailItem(t('department'), employee.Department),
        detailItem(t('problem'), record.problem, true),
        detailItem(t('assetTag'), record.asset_tag),
        detailItem(t('assignee'), record.assigned_to || 'Unassigned'),
        detailItem(t('createdBy'), record.created_by || 'Not recorded'),
        detailItem(t('priority'), record.priority),
        detailItem(t('sla'), record.sla_paused ? t('paused') : formatDate(record.sla_due_at)),
        detailItem(t('created'), formatDate(record.created_at))
      );
    }
    const history = button(t('openTimeline'), '', 'View work history and add an internal note');
    history.dataset.workActivity = kind;
    history.dataset.workId = String(record.id);
    history.dataset.workTitle = taskMode ? ('TSK-' + String(record.id).padStart(6, '0')) : (record.ticket_number || 'Ticket');
    detailActions.append(history);
    detailDialog.classList.add('is-open');
    document.body.style.overflow = 'hidden';
    detailClose.focus();
  }
  function closeDialog(dialog) {
    dialog.classList.remove('is-open');
    if (!searchDialog.classList.contains('is-open') && !detailDialog.classList.contains('is-open')) document.body.style.overflow = '';
    if (lastFocus?.isConnected) lastFocus.focus();
  }
  searchAll.addEventListener('click', () => openSearch());
  searchClose.addEventListener('click', () => closeDialog(searchDialog));
  detailClose.addEventListener('click', () => closeDialog(detailDialog));
  [searchDialog, detailDialog].forEach((dialog) => dialog.addEventListener('click', (event) => {
    if (event.target === dialog) closeDialog(dialog);
  }));
  bodyRows.addEventListener('click', (event) => {
    const trigger = event.target.closest('[data-work-details]');
    if (!trigger) return;
    const record = records.get(trigger.dataset.workDetails);
    if (record) openDetails(record, trigger);
  });
  board.addEventListener('click', (event) => {
    const trigger = event.target.closest('[data-work-details]');
    if (!trigger) return;
    const record = records.get(trigger.dataset.workDetails);
    if (record) openDetails(record, trigger);
  });

  async function runUniversalSearch() {
    const query = universalInput.value.trim();
    if (query.length < 2) {
      searchResults.replaceChildren(node('p', 'workspace-work-meta', t('searchMinimum')));
      return;
    }
    searchResults.replaceChildren(node('p', 'workspace-work-meta', t('searching')));
    try {
      const params = new URLSearchParams({ q: query, limit: '8' });
      const [ticketData, taskData] = await Promise.all([
        api('/api/tickets?' + params.toString()),
        api('/api/tasks?' + params.toString())
      ]);
      const results = [
        ...(ticketData.tickets || []).map((item) => ({ kind: 'ticket', item })),
        ...(taskData.tasks || []).map((item) => ({ kind: 'task', item }))
      ].slice(0, 16);
      searchResults.replaceChildren();
      if (!results.length) {
        searchResults.append(node('p', 'workspace-work-meta', t('searchEmpty')));
        return;
      }
      results.forEach(({ kind: resultKind, item }) => {
        const result = button('', 'workspace-search-result');
        const text = node('span');
        const heading = resultKind === 'ticket'
          ? ((item.ticket_number || 'Ticket') + ' · ' + (item.employee?.Name || 'Employee'))
          : ((item.title || 'Task') + ' · TSK-' + String(item.id).padStart(6, '0'));
        const summary = resultKind === 'ticket' ? (item.problem || '') : (item.description || '');
        text.append(node('strong', '', heading), node('small', '', statusText(item.status) + ' · ' + summary + ' · ' + formatDate(item.created_at)));
        result.append(text, node('span', 'workspace-lane-count', resultKind === 'ticket' ? 'TICKET' : 'TASK'));
        result.addEventListener('click', () => {
          const day = localDay(item.created_at);
          const searchTerm = resultKind === 'ticket' ? (item.ticket_number || query) : (item.title || query);
          const params = new URLSearchParams({ day, q: searchTerm });
          location.href = (resultKind === 'ticket' ? '/admin' : '/admin/tasks') + '?' + params.toString();
        });
        searchResults.append(result);
      });
    } catch (error) {
      searchResults.replaceChildren(node('p', 'workspace-meta-overdue', error.message || t('searchFailed')));
    }
  }
  function openSearch() {
    lastFocus = document.activeElement;
    searchDialog.classList.add('is-open');
    document.body.style.overflow = 'hidden';
    universalInput.value = '';
    searchResults.replaceChildren(node('p', 'workspace-work-meta', t('searchInitial')));
    universalInput.focus();
  }
  universalInput.addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(runUniversalSearch, 220);
  });
  universalInput.addEventListener('keydown', (event) => {
    if (event.key === 'ArrowDown') {
      const firstResult = searchResults.querySelector('button');
      if (firstResult) { event.preventDefault(); firstResult.focus(); }
    }
  });

  document.addEventListener('keydown', (event) => {
    const modifier = event.ctrlKey || event.metaKey;
    const target = event.target;
    const typing = target && (target.matches('input, textarea, select') || target.isContentEditable);
    if (modifier && event.key.toLowerCase() === 'k') {
      event.preventDefault();
      openSearch();
      return;
    }
    if (!typing && event.key === '/') {
      event.preventDefault();
      $(ids.search).focus();
      return;
    }
    if (event.key === 'Escape') {
      if (searchDialog.classList.contains('is-open')) closeDialog(searchDialog);
      else if (detailDialog.classList.contains('is-open')) closeDialog(detailDialog);
      else if (columnsMenu.classList.contains('is-open')) {
        columnsMenu.classList.remove('is-open'); columnsButton.setAttribute('aria-expanded', 'false'); columnsButton.focus();
      }
    }
    const activeDialog = searchDialog.classList.contains('is-open') ? searchDialog : detailDialog.classList.contains('is-open') ? detailDialog : null;
    if (activeDialog && event.key === 'Tab') {
      const focusable = [...activeDialog.querySelectorAll('button, input, select, a[href], [tabindex]:not([tabindex="-1"])')].filter((item) => !item.disabled);
      if (!focusable.length) return;
      if (event.shiftKey && document.activeElement === focusable[0]) { event.preventDefault(); focusable.at(-1).focus(); }
      else if (!event.shiftKey && document.activeElement === focusable.at(-1)) { event.preventDefault(); focusable[0].focus(); }
    }
  });

  const savedTextSize = Number(storageGet('ost-text-scale', 100));
  if (savedTextSize > 100) document.documentElement.classList.add('workspace-large-text');
  if (savedTextSize < 100) document.documentElement.classList.add('workspace-small-text');
  const highContrast = storageGet('ost-high-contrast', false);
  if (highContrast) document.documentElement.classList.add('workspace-high-contrast');
  largerText.addEventListener('click', () => {
    const next = Math.min(120, Number(storageGet('ost-text-scale', 100)) + 5);
    storageSet('ost-text-scale', next);
    document.documentElement.classList.toggle('workspace-large-text', next > 100);
    document.documentElement.classList.toggle('workspace-small-text', next < 100);
  });
  smallerText.addEventListener('click', () => {
    const next = Math.max(90, Number(storageGet('ost-text-scale', 100)) - 5);
    storageSet('ost-text-scale', next);
    document.documentElement.classList.toggle('workspace-large-text', next > 100);
    document.documentElement.classList.toggle('workspace-small-text', next < 100);
  });
  contrastButton.setAttribute('aria-pressed', String(highContrast));
  contrastButton.addEventListener('click', () => {
    const next = !document.documentElement.classList.contains('workspace-high-contrast');
    document.documentElement.classList.toggle('workspace-high-contrast', next);
    storageSet('ost-high-contrast', next);
    contrastButton.setAttribute('aria-pressed', String(next));
  });

  function applyLocale() {
    document.documentElement.lang = locale;
    languageButton.textContent = t('language');
    searchAll.querySelector('span').textContent = t('searchAll');
    listViewButton.textContent = t('list');
    boardViewButton.textContent = t('kanban');
    columnsButton.textContent = t('columns');
    contrastButton.textContent = t('contrast');
    savedLabel.textContent = t('savedViews');
    savedName.placeholder = t('nameView');
    saveViewButton.textContent = t('saveView');
    deleteViewButton.textContent = t('remove');
    bulkStatusButton.textContent = t('applyStatus');
    bulkOwnerButton.textContent = t('applyAssignee');
    clearSelectionButton.textContent = t('clearSelection');
    bulkStatus.options[0].textContent = t('statusPrompt');
    bulkOwner.options[0].textContent = t('assignPrompt');
    statusOptions.forEach(([value, labelKey], index) => { bulkStatus.options[index + 1].textContent = t(labelKey); });
    searchTitle.textContent = t('searchTitle');
    searchSubtitle.textContent = t('searchSubtitle');
    universalInput.placeholder = t('searchPlaceholder');
    detailTitle.textContent = t('details');
    detailSubtitle.textContent = t('detailsSubtitle');
    statuses.forEach((status) => { document.getElementById('workspaceLane-' + status.id).textContent = t(status.title); });
    const statusFilter = $(ids.status);
    if (statusFilter) {
      if (taskMode) statusFilter.options[0].textContent = locale === 'bn' ? 'সব স্ট্যাটাস' : 'All statuses';
      else statusFilter.options[0].textContent = locale === 'bn' ? 'সব স্ট্যাটাস' : 'All statuses';
      [...statusFilter.options].slice(1).forEach((option) => {
        const entry = statusOptions.find((item) => item[0] === option.value);
        if (entry) option.textContent = t(entry[1]);
      });
    }
    $(ids.search).placeholder = taskMode
      ? (locale === 'bn' ? 'বিবরণ, প্রদানকারী বা দায়িত্বপ্রাপ্ত খুঁজুন…' : 'Search details, Given by, owner…')
      : (locale === 'bn' ? 'নাম / ID / টিকিট নম্বর' : 'Name / ID / ticket no.');
    const exportButton = $(taskMode ? 'taskExportCSV' : 'ticketExportCSV');
    if (exportButton) exportButton.textContent = locale === 'bn' ? 'CSV ডাউনলোড' : 'Export CSV';
    const dateToday = $(taskMode ? 'taskToday' : 'ticketDateScopeToggle');
    if (dateToday) dateToday.textContent = locale === 'bn' ? 'আজ' : 'Today';
    const pageTitle = document.querySelector('main > header h1');
    if (pageTitle) pageTitle.textContent = t(taskMode ? 'taskAdmin' : 'ticketAdmin');
    const queueTitle = taskMode ? $('taskQueueTitle') : $('listSummary')?.parentElement?.querySelector('h2');
    if (queueTitle) queueTitle.textContent = t(taskMode ? 'taskList' : 'ticketList');
    const headerLabels = taskMode
      ? (locale === 'bn' ? ['কাজের বিবরণ', 'প্রদানকারী', 'দায়িত্ব দিন', 'অবস্থা', 'কাজ'] : ['Task details', 'Given by', 'Assign to', 'Status', 'Action'])
      : (locale === 'bn' ? ['টিকিট', 'কর্মী', 'বিভাগ', 'সমস্যা', 'Asset tag', 'Priority / SLA', 'Assignee', 'Created by', 'তৈরির সময়', 'Status', 'Action'] : ['Ticket', 'Employee', 'Department', 'Problem', 'Asset tag', 'Priority / SLA', 'Assignee', 'Created by', 'Created', 'Status', 'Action']);
    table.querySelectorAll('thead th').forEach((header, index) => { if (headerLabels[index]) header.textContent = headerLabels[index]; });
    const summaryLabels = taskMode
      ? [t('totalTasks'), t('pendingCount'), t('progressCount'), t('doneCount')]
      : [t('totalTickets'), t('pendingCount'), t('doneCount'), t('progressCount'), t('overdueCount'), t('dueSoonCount')];
    const summaryRoot = document.querySelector(taskMode ? '[aria-label="Task status totals"]' : '[aria-label="Ticket summary"]');
    const countSelector = 'strong, #totalCount, #pendingCount, #doneCount, #progressCount, #inProgressCount, #slaOverdueCount, #slaDueSoonCount';
    if (summaryRoot) summaryRoot.querySelectorAll(countSelector).forEach((count) => {
      const index = [...summaryRoot.querySelectorAll(countSelector)].indexOf(count);
      const label = count.parentElement?.querySelector('p');
      if (label && summaryLabels[index]) label.textContent = summaryLabels[index];
    });
    const refreshButton = $(taskMode ? 'taskAdminRefresh' : 'refreshBtn');
    if (refreshButton && !refreshButton.disabled) refreshButton.textContent = locale === 'bn' ? 'রিফ্রেশ' : 'Refresh';
    bodyRows.querySelectorAll('select').forEach((select) => {
      [...select.options].forEach((option) => {
        const entry = statusOptions.find((item) => item[0] === option.value);
        if (entry) option.textContent = t(entry[1]);
      });
    });
    if (taskMode) bodyRows.querySelectorAll('tr[data-workspace-id]').forEach((row) => {
      const record = records.get(row.dataset.workspaceId);
      if (!record) return;
      row.querySelectorAll('[data-workspace-meta="priority"]').forEach((item) => { item.textContent = t('priority') + ' · ' + (record.priority || 'medium'); });
      row.querySelectorAll('[data-workspace-meta="due"]').forEach((item) => {
        item.textContent = t('due') + ' · ' + (record.due_date || t('noDate')) + (isOverdue(record) ? ' · ' + t('overdue') : '');
      });
    });
    bodyRows.querySelectorAll('button').forEach((item) => {
      const text = item.textContent.trim();
      if (text === 'History' || text === 'ইতিহাস') item.textContent = t('history');
      else if (text === 'Save' || text === 'সংরক্ষণ') item.textContent = t('save');
      else if (text === 'Done করুন' || text === 'Done') item.textContent = t('markDone');
      else if (text === 'Details' || text === 'বিবরণ') item.textContent = t('detailsAction');
    });
    refreshSavedViews();
    syncSelection();
    renderBoard([...records.values()]);
  }
  languageButton.addEventListener('click', () => {
    locale = locale === 'en' ? 'bn' : 'en';
    try { localStorage.setItem('ost-admin-locale', locale); } catch (_) {}
    applyLocale();
  });
  applyLocale();

  table.setAttribute('aria-label', taskMode ? 'Tasks' : 'Tickets');
  table.setAttribute('aria-describedby', bulkMessage.id);
  setView(storageGet('ost-view-' + kind, 'list') === 'board' ? 'board' : 'list');
})();
