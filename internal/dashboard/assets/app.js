const reduceMotionQuery = typeof window.matchMedia === 'function'
  ? window.matchMedia('(prefers-reduced-motion: reduce)')
  : null;

const state = {
  snapshot: null,
  uploadHistory: Array(60).fill(0),
  downloadHistory: Array(60).fill(0),
  lastTickAt: performance.now(),
  frameHandle: 0,
  prefersReducedMotion: reduceMotionQuery ? reduceMotionQuery.matches : false,
  announceTimer: 0,
  lastIfaceKeys: '',
};

const elements = {
  healthDot: document.getElementById('health-dot'),
  heroStatus: document.getElementById('hero-status'),
  rateUp: document.getElementById('rate-up'),
  rateDown: document.getElementById('rate-down'),
  mode: document.getElementById('mode'),
  interfacesCount: document.getElementById('interfaces-count'),
  autoProxy: document.getElementById('auto-proxy'),
  totalUpload: document.getElementById('total-upload'),
  totalDownload: document.getElementById('total-download'),
  httpProxy: document.getElementById('http-proxy'),
  socks5Proxy: document.getElementById('socks5-proxy'),
  activeConnections: document.getElementById('active-connections'),
  totalConnections: document.getElementById('total-connections'),
  uptime: document.getElementById('uptime'),
  version: document.getElementById('version'),
  interfacesMeta: document.getElementById('interfaces-meta'),
  interfacesList: document.getElementById('interfaces-list'),
  footerText: document.getElementById('footer-text'),
  errorBanner: document.getElementById('error-banner'),
  graphStatus: document.getElementById('graph-status'),
  graphRange: document.getElementById('graph-range'),
  legendUp: document.getElementById('legend-up'),
  legendDown: document.getElementById('legend-down'),
  graphEmpty: document.getElementById('graph-empty'),
  graphEmptyTitle: document.getElementById('graph-empty-title'),
  graphEmptyCopy: document.getElementById('graph-empty-copy'),
  chartDescription: document.getElementById('chart-description'),
  canvas: document.getElementById('speed-chart'),
  resetButton: document.getElementById('reset-stats'),
  announcer: document.getElementById('announcer'),
};

function modeLabel(mode) {
  return mode === 1 ? 'Failover' : 'Round-Robin';
}

function formatBytes(value) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let size = Number(value || 0);
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  if (unitIndex === 0) {
    return `${Math.round(size)} ${units[unitIndex]}`;
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[unitIndex]}`;
}

function formatRate(value) {
  return `${formatBytes(value)}/s`;
}

function formatDuration(duration) {
  const totalSeconds = Math.max(0, Math.floor(Number(duration || 0) / 1e9));
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const parts = [];
  if (days) parts.push(`${days}d`);
  if (hours || parts.length) parts.push(`${hours}h`);
  if (minutes || parts.length) parts.push(`${minutes}m`);
  if (!parts.length) parts.push(`${seconds}s`);
  return parts.slice(0, 3).join(' ');
}

function formatVersion(value) {
  if (!value) {
    return '—';
  }
  const version = String(value);
  return version.startsWith('v') ? version : `v${version}`;
}

function nonEmpty(value, fallback = '—') {
  return value ? String(value) : fallback;
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function appendHistory(series, value) {
  series.push(Number(value || 0));
  while (series.length > 60) {
    series.shift();
  }
}

function updateHistory(snapshot) {
  appendHistory(state.uploadHistory, snapshot.rate_sent);
  appendHistory(state.downloadHistory, snapshot.rate_recv);
  state.lastTickAt = performance.now();
}

function currentMaxTraffic() {
  return Math.max(0, ...state.uploadHistory, ...state.downloadHistory);
}

function announce(message) {
  if (!message || !elements.announcer) {
    return;
  }
  window.clearTimeout(state.announceTimer);
  elements.announcer.textContent = '';
  state.announceTimer = window.setTimeout(() => {
    elements.announcer.textContent = message;
  }, 20);
}

function announceImportantChanges(previous, snapshot) {
  if (!previous || !snapshot) {
    return;
  }

  const previousError = previous.last_error ? String(previous.last_error) : '';
  const nextError = snapshot.last_error ? String(snapshot.last_error) : '';
  if (previousError !== nextError) {
    announce(nextError ? `Latest error: ${nextError}` : 'Latest error cleared.');
    return;
  }

  if (!!previous.running !== !!snapshot.running) {
    announce(snapshot.running ? 'Proxy running.' : 'Proxy stopped.');
  }
}

function setErrorState(message) {
  if (message) {
    elements.errorBanner.textContent = message;
    elements.errorBanner.classList.remove('hidden');
    elements.healthDot.classList.add('error');
  } else {
    elements.errorBanner.textContent = '';
    elements.errorBanner.classList.add('hidden');
    elements.healthDot.classList.remove('error');
  }
}

function renderInterfaces(interfaces = []) {
  const selected = interfaces.filter((item) => item.selected);
  elements.interfacesMeta.textContent = `${selected.length} selected interface${selected.length === 1 ? '' : 's'}`;

  if (!selected.length) {
    state.lastIfaceKeys = '';
    elements.interfacesList.innerHTML = '<div class="empty-state">No interfaces selected. Use the tray menu to choose which adapters contribute traffic.</div>';
    return;
  }

  const currentKeys = selected.map((iface) => iface.key).join(',');
  const needsRebuild = currentKeys !== state.lastIfaceKeys;

  if (needsRebuild) {
    state.lastIfaceKeys = currentKeys;
    elements.interfacesList.innerHTML = selected.map((iface) => {
      const label = iface.friendly_name || iface.name || 'Unknown interface';
      const ip = nonEmpty(iface.ip);
      const networkName = iface.network_name ? escapeHtml(iface.network_name) : '';
      const statusClass = iface.alive ? '' : ' down';
      const statusText = iface.alive ? 'Online' : 'Offline';
      const subline = networkName
        ? `<span data-el="network-name">${networkName}</span> • <span data-el="ip">${escapeHtml(ip)}</span> • <span data-el="status-text">${statusText}</span>`
        : `<span data-el="ip">${escapeHtml(ip)}</span> • <span data-el="status-text">${statusText}</span>`;
      return `
        <article class="interface-card" data-key="${escapeHtml(iface.key)}">
          <div class="interface-card__name">
            <span class="interface-status${statusClass}" data-el="status"></span>
            <div>
              <div class="interface-card__label">${escapeHtml(label)}</div>
              <div class="interface-card__subline">${subline}</div>
            </div>
          </div>
          <div class="interface-card__rate interface-card__rate--up">
            <span>↑</span><strong data-el="rate-up">${formatRate(iface.rate_sent)}</strong>
          </div>
          <div class="interface-card__total">
            Total ↑ <strong data-el="total-up">${formatBytes(iface.bytes_sent)}</strong>
          </div>
          <div class="interface-card__rate interface-card__rate--down">
            <span>↓</span><strong data-el="rate-down">${formatRate(iface.rate_recv)}</strong>
          </div>
          <div class="interface-card__total">
            Total ↓ <strong data-el="total-down">${formatBytes(iface.bytes_recv)}</strong>
          </div>
        </article>`;
    }).join('');
    return;
  }

  // Diff update: only change text content of existing cards
  selected.forEach((iface) => {
    const card = elements.interfacesList.querySelector(`[data-key="${CSS.escape(iface.key)}"]`);
    if (!card) return;

    const statusDot = card.querySelector('[data-el="status"]');
    const statusTextEl = card.querySelector('[data-el="status-text"]');
    const ipEl = card.querySelector('[data-el="ip"]');
    const networkNameEl = card.querySelector('[data-el="network-name"]');
    const rateUpEl = card.querySelector('[data-el="rate-up"]');
    const rateDownEl = card.querySelector('[data-el="rate-down"]');
    const totalUpEl = card.querySelector('[data-el="total-up"]');
    const totalDownEl = card.querySelector('[data-el="total-down"]');

    if (statusDot) {
      statusDot.className = iface.alive ? 'interface-status' : 'interface-status down';
    }
    if (statusTextEl) statusTextEl.textContent = iface.alive ? 'Online' : 'Offline';
    if (ipEl) ipEl.textContent = nonEmpty(iface.ip);
    if (iface.network_name && !networkNameEl) {
      state.lastIfaceKeys = '';
      renderInterfaces(interfaces);
      return;
    }
    if (!iface.network_name && networkNameEl) {
      state.lastIfaceKeys = '';
      renderInterfaces(interfaces);
      return;
    }
    if (networkNameEl) networkNameEl.textContent = iface.network_name;
    if (rateUpEl) rateUpEl.textContent = formatRate(iface.rate_sent);
    if (rateDownEl) rateDownEl.textContent = formatRate(iface.rate_recv);
    if (totalUpEl) totalUpEl.textContent = formatBytes(iface.bytes_sent);
    if (totalDownEl) totalDownEl.textContent = formatBytes(iface.bytes_recv);
  });
}

function updateGraphMeta(snapshot) {
  const maxTraffic = currentMaxTraffic();
  elements.legendUp.textContent = formatRate(snapshot ? snapshot.rate_sent : 0);
  elements.legendDown.textContent = formatRate(snapshot ? snapshot.rate_recv : 0);
  elements.graphRange.textContent = state.prefersReducedMotion
    ? 'Last 60 seconds · Reduced motion'
    : 'Last 60 seconds';

  if (!snapshot) {
    elements.graphStatus.textContent = 'Waiting for live traffic data from the tray.';
    elements.graphEmptyTitle.textContent = 'Waiting for live traffic data.';
    elements.graphEmptyCopy.textContent = 'The tray keeps collecting stats even while this window is hidden.';
    elements.graphEmpty.classList.remove('hidden');
    elements.chartDescription.textContent = 'Traffic chart waiting for data from the tray.';
    return;
  }

  if (maxTraffic <= 0) {
    elements.graphStatus.textContent = 'No traffic in the last minute yet.';
    elements.graphEmptyTitle.textContent = 'No traffic in the last 60 seconds.';
    elements.graphEmptyCopy.textContent = 'Generate traffic or wait for the next transfer to see the chart fill in.';
    elements.graphEmpty.classList.remove('hidden');
    elements.chartDescription.textContent = 'Traffic chart showing no transfer activity in the last 60 seconds.';
    return;
  }

  elements.graphStatus.textContent = state.prefersReducedMotion
    ? 'Live traffic from the tray with motion reduced.'
    : 'Live traffic from the tray.';
  elements.graphEmpty.classList.add('hidden');
  elements.chartDescription.textContent = `Traffic chart for the last 60 seconds. Current upload ${formatRate(snapshot.rate_sent)} and download ${formatRate(snapshot.rate_recv)}.`;
}

function buildHeroStatus(snapshot) {
  if (snapshot.last_error) {
    return 'Attention needed. Proxy controls stay available in the tray.';
  }
  if (snapshot.running) {
    const count = snapshot.active_interfaces || 0;
    return `Proxy running • ${count} active interface${count === 1 ? '' : 's'}`;
  }
  return 'Proxy stopped. Use the tray menu to review connectivity.';
}

function buildFooterText(snapshot) {
  if (snapshot && snapshot.last_error) {
    return 'Close this window to hide it. Tray controls stay available while the latest error is shown.';
  }
  return 'Close this window to hide it. Left-click the tray icon to bring it back.';
}

function renderSnapshot() {
  const snapshot = state.snapshot;
  updateGraphMeta(snapshot);
  if (!snapshot) {
    return;
  }

  elements.heroStatus.textContent = buildHeroStatus(snapshot);
  elements.rateUp.textContent = formatRate(snapshot.rate_sent);
  elements.rateDown.textContent = formatRate(snapshot.rate_recv);
  elements.mode.textContent = modeLabel(snapshot.mode);
  elements.interfacesCount.textContent = `${snapshot.active_interfaces || 0} active`;
  elements.autoProxy.textContent = snapshot.win_proxy_auto ? 'Enabled' : 'Disabled';
  elements.totalUpload.textContent = formatBytes(snapshot.bytes_sent);
  elements.totalDownload.textContent = formatBytes(snapshot.bytes_recv);
  elements.httpProxy.textContent = nonEmpty(snapshot.proxy_addr);
  elements.socks5Proxy.textContent = nonEmpty(snapshot.socks5_addr);
  elements.activeConnections.textContent = `${snapshot.active_connections || 0}`;
  elements.totalConnections.textContent = `${snapshot.total_connections || 0}`;
  elements.uptime.textContent = formatDuration(snapshot.uptime);
  elements.version.textContent = formatVersion(snapshot.version);
  elements.footerText.textContent = buildFooterText(snapshot);
  setErrorState(snapshot.last_error);
  renderInterfaces(snapshot.interfaces);
}

function resizeCanvas() {
  const dpr = window.devicePixelRatio || 1;
  const rect = elements.canvas.getBoundingClientRect();
  const width = Math.max(1, Math.floor(rect.width * dpr));
  const height = Math.max(1, Math.floor(rect.height * dpr));
  if (elements.canvas.width !== width || elements.canvas.height !== height) {
    elements.canvas.width = width;
    elements.canvas.height = height;
  }
}

function drawGrid(ctx, width, height, padding) {
  ctx.save();
  ctx.strokeStyle = 'rgba(139, 148, 158, 0.12)';
  ctx.lineWidth = 1;
  for (let i = 0; i <= 4; i += 1) {
    const y = padding.top + ((height - padding.top - padding.bottom) / 4) * i;
    ctx.beginPath();
    ctx.moveTo(padding.left, y);
    ctx.lineTo(width - padding.right, y);
    ctx.stroke();
  }
  ctx.restore();
}

function drawSeries(ctx, values, options) {
  const { width, height, padding, color, fillColor, offset, maxValue } = options;
  const innerWidth = width - padding.left - padding.right;
  const innerHeight = height - padding.top - padding.bottom;
  const step = innerWidth / Math.max(1, values.length - 1);

  ctx.save();
  ctx.beginPath();
  values.forEach((value, index) => {
    const x = padding.left + (step * index) - offset;
    const y = padding.top + innerHeight - ((value / maxValue) * innerHeight);
    if (index === 0) {
      ctx.moveTo(x, y);
    } else {
      ctx.lineTo(x, y);
    }
  });
  ctx.lineWidth = 3;
  ctx.strokeStyle = color;
  ctx.shadowColor = color;
  ctx.shadowBlur = 14;
  ctx.stroke();

  ctx.lineTo(padding.left + innerWidth - offset, height - padding.bottom);
  ctx.lineTo(padding.left - offset, height - padding.bottom);
  ctx.closePath();
  const gradient = ctx.createLinearGradient(0, padding.top, 0, height - padding.bottom);
  gradient.addColorStop(0, fillColor);
  gradient.addColorStop(1, 'rgba(0, 0, 0, 0)');
  ctx.fillStyle = gradient;
  ctx.fill();
  ctx.restore();
}

function drawLabels(ctx, width, height, padding, maxValue) {
  ctx.save();
  ctx.fillStyle = 'rgba(139, 148, 158, 0.92)';
  ctx.font = '12px Segoe UI';
  ctx.fillText(formatRate(maxValue > 0 ? maxValue : 0), padding.left, padding.top - 8);
  ctx.fillText('0 B/s', padding.left, height - 8);
  const label = 'Last 60 seconds';
  const measure = ctx.measureText(label);
  ctx.fillText(label, width - padding.right - measure.width, height - 8);
  ctx.restore();
}

function drawGraph() {
  resizeCanvas();
  const ctx = elements.canvas.getContext('2d');
  if (!ctx) {
    return;
  }

  const dpr = window.devicePixelRatio || 1;
  const width = elements.canvas.width;
  const height = elements.canvas.height;
  const padding = {
    top: 18 * dpr,
    right: 16 * dpr,
    bottom: 18 * dpr,
    left: 16 * dpr,
  };
  const maxTraffic = currentMaxTraffic();
  const chartMax = Math.max(1, maxTraffic);

  ctx.clearRect(0, 0, width, height);
  drawGrid(ctx, width, height, padding);

  if (maxTraffic > 0) {
    const innerWidth = width - padding.left - padding.right;
    const step = innerWidth / Math.max(1, state.uploadHistory.length - 1);
    const progress = state.prefersReducedMotion
      ? 0
      : Math.min(1, (performance.now() - state.lastTickAt) / 1000);
    const offset = progress * step;

    drawSeries(ctx, state.downloadHistory, {
      width,
      height,
      padding,
      color: '#d2a8ff',
      fillColor: 'rgba(210, 168, 255, 0.18)',
      offset,
      maxValue: chartMax,
    });
    drawSeries(ctx, state.uploadHistory, {
      width,
      height,
      padding,
      color: '#58a6ff',
      fillColor: 'rgba(88, 166, 255, 0.18)',
      offset,
      maxValue: chartMax,
    });
  }

  drawLabels(ctx, width, height, padding, chartMax);
}

function stopFrame() {
  if (state.frameHandle) {
    window.cancelAnimationFrame(state.frameHandle);
    state.frameHandle = 0;
  }
}

function syncAnimationLoop() {
  const shouldAnimate = !state.prefersReducedMotion && !document.hidden && currentMaxTraffic() > 0;
  if (!shouldAnimate) {
    stopFrame();
    drawGraph();
    return;
  }
  if (state.frameHandle) {
    return;
  }
  const frame = () => {
    state.frameHandle = window.requestAnimationFrame(frame);
    drawGraph();
  };
  state.frameHandle = window.requestAnimationFrame(frame);
}

window.pushSnapshot = function pushSnapshot(snapshot) {
  if (typeof snapshot === 'string') {
    snapshot = JSON.parse(snapshot);
  }
  const previous = state.snapshot;
  state.snapshot = snapshot || {};
  updateHistory(state.snapshot);
  renderSnapshot();
  drawGraph();
  syncAnimationLoop();
  announceImportantChanges(previous, state.snapshot);
};

window.resetDashboardView = function resetDashboardView() {
  state.uploadHistory = Array(60).fill(0);
  state.downloadHistory = Array(60).fill(0);
  updateGraphMeta(state.snapshot);
  drawGraph();
  syncAnimationLoop();
};

window.addEventListener('resize', drawGraph);
document.addEventListener('visibilitychange', syncAnimationLoop);

if (reduceMotionQuery) {
  const handleMotionChange = (event) => {
    state.prefersReducedMotion = !!event.matches;
    updateGraphMeta(state.snapshot);
    syncAnimationLoop();
  };

  if (typeof reduceMotionQuery.addEventListener === 'function') {
    reduceMotionQuery.addEventListener('change', handleMotionChange);
  } else if (typeof reduceMotionQuery.addListener === 'function') {
    reduceMotionQuery.addListener(handleMotionChange);
  }
}

elements.resetButton.addEventListener('click', async () => {
  if (typeof window.resetStats !== 'function') {
    return;
  }
  try {
    await window.resetStats();
    window.resetDashboardView();
    announce('Statistics reset.');
  } catch (error) {
    const message = error && error.message ? error.message : String(error);
    setErrorState(`Reset stats failed: ${message}`);
    announce(`Reset stats failed: ${message}`);
  }
});

renderSnapshot();
drawGraph();
syncAnimationLoop();
if (typeof window.__dashboardReady === 'function') {
  window.__dashboardReady();
}
