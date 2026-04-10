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
  systemStatus: document.getElementById('system-status'),
  systemStatusPill: document.getElementById('system-status-pill'),
  heroStatus: document.getElementById('hero-status'),
  navRateUp: document.getElementById('nav-rate-up'),
  navRateDown: document.getElementById('nav-rate-down'),
  mode: document.getElementById('mode'),
  interfacesCount: document.getElementById('interfaces-count'),
  autoProxy: document.getElementById('auto-proxy'),
  autoProxyToggle: document.getElementById('auto-proxy-toggle'),
  totalUpload: document.getElementById('total-upload'),
  totalDownload: document.getElementById('total-download'),
  httpProxy: document.getElementById('http-proxy'),
  socks5Proxy: document.getElementById('socks5-proxy'),
  activeConnections: document.getElementById('active-connections'),
  totalConnections: document.getElementById('total-connections'),
  uptime: document.getElementById('uptime'),
  version: document.getElementById('version'),
  footerUptime: document.getElementById('footer-uptime'),
  footerVersion: document.getElementById('footer-version'),
  interfacesMeta: document.getElementById('interfaces-meta'),
  interfacesList: document.getElementById('interfaces-list'),
  footerText: document.getElementById('footer-text'),
  errorBanner: document.getElementById('error-banner'),
  graphStatus: document.getElementById('graph-status'),
  legendUp: document.getElementById('legend-up'),
  legendDown: document.getElementById('legend-down'),
  graphAxisMax: document.getElementById('graph-axis-max'),
  graphAxisMid: document.getElementById('graph-axis-mid'),
  graphEmpty: document.getElementById('graph-empty'),
  graphEmptyTitle: document.getElementById('graph-empty-title'),
  graphEmptyCopy: document.getElementById('graph-empty-copy'),
  chartDescription: document.getElementById('chart-description'),
  canvas: document.getElementById('speed-chart'),
  resetButton: document.getElementById('reset-stats'),
  announcer: document.getElementById('announcer'),
};

function modeLabel(mode) {
  return mode === 1 ? 'FAILOVER' : 'ROUND-ROBIN';
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

  if (days > 0) {
    return [days, hours, minutes, seconds]
      .map((part) => String(part).padStart(2, '0'))
      .join(':');
  }

  return [hours, minutes, seconds]
    .map((part) => String(part).padStart(2, '0'))
    .join(':');
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

function interfaceIcon(iface) {
  const label = `${iface.friendly_name || ''} ${iface.name || ''} ${iface.network_name || ''}`.toLowerCase();
  if (label.includes('wi-fi') || label.includes('wifi') || label.includes('wlan') || label.includes('ssid')) {
    return { icon: 'wifi', tint: 'primary' };
  }
  if (label.includes('ethernet') || label.includes('eth')) {
    return { icon: 'settings_ethernet', tint: 'secondary' };
  }
  if (label.includes('vpn') || label.includes('wireguard') || label.includes('tun')) {
    return { icon: 'vpn_lock', tint: 'muted' };
  }
  if (label.includes('wwan') || label.includes('lte') || label.includes('mobile') || label.includes('cell')) {
    return { icon: 'cell_tower', tint: 'primary' };
  }
  return { icon: 'cable', tint: 'primary' };
}

function buildStatusTone(snapshot) {
  if (snapshot.last_error) {
    return 'error';
  }
  if (snapshot.running) {
    return 'running';
  }
  return 'stopped';
}

function buildSystemStatusLabel(snapshot) {
  if (snapshot.last_error) {
    return 'SYSTEM_ALERT';
  }
  if (snapshot.running) {
    return 'SYSTEM_ONLINE';
  }
  return 'PROXY_STOPPED';
}

function buildHeroStatus(snapshot) {
  if (snapshot.last_error) {
    return 'Attention needed. Tray controls remain available.';
  }
  if (snapshot.running) {
    return `Proxy running across ${snapshot.active_interfaces || 0} active interface${snapshot.active_interfaces === 1 ? '' : 's'}.`;
  }
  return 'Proxy stopped. Use the tray menu to review connectivity.';
}

function buildFooterText(snapshot) {
  if (snapshot && snapshot.last_error) {
    return 'Close this window to hide it. Tray controls stay available while the latest error is shown.';
  }
  return 'Close this window to hide it. Left-click the tray icon to bring it back.';
}

function setStatusTone(snapshot) {
  const tone = buildStatusTone(snapshot);
  elements.healthDot.dataset.state = tone;
  elements.systemStatusPill.dataset.state = tone;
  elements.systemStatus.textContent = buildSystemStatusLabel(snapshot);
}

function setErrorState(message) {
  if (message) {
    elements.errorBanner.textContent = message;
    elements.errorBanner.classList.remove('hidden');
  } else {
    elements.errorBanner.textContent = '';
    elements.errorBanner.classList.add('hidden');
  }
}

function renderInterfaces(interfaces = []) {
  const active = interfaces.filter((item) => item.alive);
  elements.interfacesMeta.textContent = `${active.length} ACTIVE`;

  if (!active.length) {
    state.lastIfaceKeys = '';
    elements.interfacesList.innerHTML = '<div class="pm-empty-state">No active network interfaces detected.</div>';
    return;
  }

  const currentKeys = active.map((iface) => iface.key).join(',');
  const needsRebuild = currentKeys !== state.lastIfaceKeys;

  if (needsRebuild) {
    state.lastIfaceKeys = currentKeys;
    elements.interfacesList.innerHTML = active.map((iface) => {
      const label = escapeHtml(iface.friendly_name || iface.name || 'Unknown interface');
      const networkName = escapeHtml(iface.network_name || iface.gateway || 'Connected');
      const ip = escapeHtml(nonEmpty(iface.ip));
      const ratesUp = formatRate(iface.rate_sent);
      const ratesDown = formatRate(iface.rate_recv);
      const { icon, tint } = interfaceIcon(iface);
      const iconClass = tint === 'secondary'
        ? 'pm-interface-icon text-secondary bg-secondary/10 border-secondary/20'
        : tint === 'muted'
          ? 'pm-interface-icon text-gray-500 bg-gray-500/10 border-gray-500/20'
          : 'pm-interface-icon text-primary bg-primary/10 border-primary/20';

      return `
        <article class="pm-interface-card" data-key="${escapeHtml(iface.key)}">
          <div class="${iconClass}">
            <span class="material-symbols-outlined" aria-hidden="true">${icon}</span>
          </div>
          <div class="pm-interface-body">
            <div class="pm-interface-ident">
              <div class="pm-interface-name" data-el="name">${label}</div>
              <div class="pm-interface-network" data-el="network">${networkName}</div>
            </div>
            <div class="pm-interface-ip">
              <div class="pm-interface-ip-label">IP_ADDR</div>
              <div class="pm-interface-ip-value" data-el="ip">${ip}</div>
            </div>
            <div class="pm-interface-rates">
              <span class="pm-interface-rate pm-interface-rate--up" data-el="rate-up">↑ ${ratesUp}</span>
              <span class="pm-interface-rate pm-interface-rate--down" data-el="rate-down">↓ ${ratesDown}</span>
            </div>
          </div>
        </article>`;
    }).join('');
    return;
  }

  active.forEach((iface) => {
    const card = elements.interfacesList.querySelector(`[data-key="${CSS.escape(iface.key)}"]`);
    if (!card) {
      state.lastIfaceKeys = '';
      renderInterfaces(interfaces);
      return;
    }

    const nameEl = card.querySelector('[data-el="name"]');
    const networkEl = card.querySelector('[data-el="network"]');
    const ipEl = card.querySelector('[data-el="ip"]');
    const rateUpEl = card.querySelector('[data-el="rate-up"]');
    const rateDownEl = card.querySelector('[data-el="rate-down"]');

    if (nameEl) nameEl.textContent = iface.friendly_name || iface.name || 'Unknown interface';
    if (networkEl) networkEl.textContent = iface.network_name || iface.gateway || 'Connected';
    if (ipEl) ipEl.textContent = nonEmpty(iface.ip);
    if (rateUpEl) rateUpEl.textContent = `↑ ${formatRate(iface.rate_sent)}`;
    if (rateDownEl) rateDownEl.textContent = `↓ ${formatRate(iface.rate_recv)}`;
  });
}

function updateGraphMeta(snapshot) {
  const maxTraffic = currentMaxTraffic();
  const axisMax = formatRate(maxTraffic > 0 ? maxTraffic : 0);
  const axisMid = formatRate(maxTraffic > 0 ? maxTraffic / 2 : 0);
  elements.legendUp.textContent = formatRate(snapshot ? snapshot.rate_sent : 0);
  elements.legendDown.textContent = formatRate(snapshot ? snapshot.rate_recv : 0);
  elements.graphAxisMax.textContent = axisMax;
  elements.graphAxisMid.textContent = axisMid;

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
    ? 'Live traffic with reduced motion enabled.'
    : 'Live traffic from the tray.';
  elements.graphEmpty.classList.add('hidden');
  elements.chartDescription.textContent = `Traffic chart for the last 60 seconds. Current upload ${formatRate(snapshot.rate_sent)} and download ${formatRate(snapshot.rate_recv)}.`;
}

function renderSnapshot() {
  const snapshot = state.snapshot;
  updateGraphMeta(snapshot);
  if (!snapshot) {
    return;
  }

  setStatusTone(snapshot);
  elements.heroStatus.textContent = buildHeroStatus(snapshot);
  elements.navRateUp.textContent = formatRate(snapshot.rate_sent);
  elements.navRateDown.textContent = formatRate(snapshot.rate_recv);
  elements.mode.textContent = modeLabel(snapshot.mode);
  elements.interfacesCount.textContent = `${snapshot.active_interfaces || 0} ACTIVE`;
  elements.autoProxy.textContent = snapshot.win_proxy_auto ? 'Enabled' : 'Disabled';
  elements.autoProxyToggle.dataset.enabled = snapshot.win_proxy_auto ? 'true' : 'false';
  elements.totalUpload.textContent = formatBytes(snapshot.bytes_sent);
  elements.totalDownload.textContent = formatBytes(snapshot.bytes_recv);
  elements.httpProxy.textContent = nonEmpty(snapshot.proxy_addr);
  elements.socks5Proxy.textContent = nonEmpty(snapshot.socks5_addr);
  elements.activeConnections.textContent = `${snapshot.active_connections || 0}`;
  elements.totalConnections.textContent = `${snapshot.total_connections || 0}`;
  elements.uptime.textContent = formatDuration(snapshot.uptime);
  elements.version.textContent = formatVersion(snapshot.version);
  elements.footerUptime.textContent = formatDuration(snapshot.uptime);
  elements.footerVersion.textContent = formatVersion(snapshot.version);
  elements.footerText.textContent = buildFooterText(snapshot);
  setErrorState(snapshot.last_error);
  renderInterfaces(snapshot.interfaces || []);
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
  ctx.strokeStyle = 'rgba(255, 255, 255, 0.05)';
  ctx.lineWidth = 1;
  for (let i = 1; i <= 3; i += 1) {
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
  ctx.shadowBlur = 16;
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

function drawGraph() {
  resizeCanvas();
  const ctx = elements.canvas.getContext('2d');
  if (!ctx) {
    return;
  }

  const width = elements.canvas.width;
  const height = elements.canvas.height;
  const dpr = window.devicePixelRatio || 1;
  const padding = {
    top: 16 * dpr,
    right: 16 * dpr,
    bottom: 16 * dpr,
    left: 54 * dpr,
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
      color: '#d575ff',
      fillColor: 'rgba(213, 117, 255, 0.14)',
      offset,
      maxValue: chartMax,
    });
    drawSeries(ctx, state.uploadHistory, {
      width,
      height,
      padding,
      color: '#99f7ff',
      fillColor: 'rgba(153, 247, 255, 0.14)',
      offset,
      maxValue: chartMax,
    });
  }
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
