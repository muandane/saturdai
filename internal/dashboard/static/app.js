const NOW = new Date();

let PROFILES = [];
let selectedProfile = null;
let selectedContainer = 0;
let cpuChart = null;
let memChart = null;

async function loadProfiles() {
  const statusEl = document.getElementById('loadStatusFooter');
  statusEl.textContent = 'loading…';
  try {
    const r = await fetch('/api/dashboard/v1/profiles');
    if (!r.ok) {
      throw new Error(`HTTP ${r.status}`);
    }
    const data = await r.json();
    PROFILES = data.profiles || [];
    document.getElementById('profileCountFooter').textContent =
      PROFILES.length ? `${PROFILES.length} loaded` : 'none';
    statusEl.textContent = 'live API';
    if (PROFILES.length === 0) {
      document.getElementById('profileList').innerHTML =
        '<div style="padding:12px 14px;color:var(--txt3);font-size:11px">No WorkloadProfiles in cluster.</div>';
      document.getElementById('topName').textContent = '—';
      document.getElementById('topRef').textContent = '';
      return;
    }
    selectedProfile = PROFILES[0];
    selectedContainer = 0;
    buildSidebar();
    buildTopbar();
    renderAllPanels();
  } catch (e) {
    statusEl.textContent = 'error';
    document.getElementById('profileList').innerHTML =
      `<div style="padding:12px 14px;color:var(--err);font-size:11px">${String(e.message || e)}</div>`;
  }
}

function profileStatus(p) {
  if (!p.conditions.profileReady) return 'err';
  if (!p.conditions.metricsAvailable) return 'warn';
  return 'ok';
}

function genEMASeries(base, volatility, points = 20) {
  const short = [];
  const lng = [];
  let sv = base;
  let lv = base * 0.9;
  const alphaS = 0.3;
  const alphaL = 0.1;
  for (let i = 0; i < points; i++) {
    const raw = base + (Math.random() - 0.48) * volatility;
    sv = alphaS * raw + (1 - alphaS) * sv;
    lv = alphaL * raw + (1 - alphaL) * lv;
    short.push(+sv.toFixed(1));
    lng.push(+lv.toFixed(1));
  }
  return { short, long: lng };
}

function timeLabels(n = 20) {
  const labels = [];
  for (let i = n - 1; i >= 0; i--) {
    const d = new Date(NOW);
    d.setSeconds(d.getSeconds() - i * 30);
    labels.push(d.toISOString().slice(11, 16));
  }
  return labels;
}

function buildSidebar() {
  const el = document.getElementById('profileList');
  el.innerHTML = '';
  PROFILES.forEach((p) => {
    const st = profileStatus(p);
    const div = document.createElement('div');
    div.className = 'profile-item' + (p.id === selectedProfile.id ? ' active' : '');
    div.innerHTML = `
      <div class="profile-name">${p.name}</div>
      <div class="profile-meta">
        <span class="status-dot dot-${st}"></span>
        <span class="badge badge-${p.mode}">${p.mode}</span>
        <span class="profile-kind">${p.ns}</span>
      </div>
    `;
    div.onclick = () => selectProfile(p);
    el.appendChild(div);
  });
}

function selectProfile(p) {
  selectedProfile = p;
  selectedContainer = 0;
  buildSidebar();
  buildTopbar();
  renderAllPanels();
}

function buildTopbar() {
  const p = selectedProfile;
  document.getElementById('topName').textContent = p.name;
  document.getElementById('topRef').textContent =
    `${p.kind} / ${p.ns} / ${p.targetName} · interval ${p.interval}s · cooldown ${p.cooldown}m`;
  document.getElementById('topTs').textContent =
    p.lastEvaluated ? `evaluated ${p.lastEvaluated}` : '';
  const c = p.conditions;
  const badges = document.getElementById('condBadges');
  badges.innerHTML = '';
  const items = [
    { label: 'TargetResolved', ok: c.targetResolved },
    { label: 'MetricsAvailable', ok: c.metricsAvailable, warn: !c.metricsAvailable },
    { label: 'ProfileReady', ok: c.profileReady },
  ];
  items.forEach((item) => {
    const span = document.createElement('span');
    span.className = 'cond ' + (item.ok ? 'cond-ok' : item.warn ? 'cond-warn' : 'cond-err');
    span.innerHTML = '<span style="font-size:8px">' + (item.ok ? '●' : '✕') + '</span> ' + item.label;
    badges.appendChild(span);
  });
  if (selectedProfile.downsizePause > 0) {
    const span = document.createElement('span');
    span.className = 'cond cond-warn';
    span.innerHTML = '⏸ pause×' + selectedProfile.downsizePause;
    badges.appendChild(span);
  }
}

function renderAllPanels() {
  if (!selectedProfile || !selectedProfile.containers || !selectedProfile.containers.length) {
    return;
  }
  renderOverview();
  renderContainers();
  renderRecommendations();
  renderSignals();
  renderQuadrant();
}

function renderOverview() {
  const p = selectedProfile;
  const c = p.containers[0];
  const cards = [
    { label: 'Containers', val: p.containers.length, sub: 'pod template' },
    { label: 'CPU EMAShort', val: c.cpu.emaShort + 'm', sub: 'short window', cls: 'metric-cpu' },
    { label: 'Mem EMAShort', val: fmtMi(c.memory.emaShort), sub: 'short window', cls: 'metric-mem' },
    {
      label: 'Pause cycles',
      val: p.downsizePause || '—',
      sub: 'downsize blocked',
      cls: p.downsizePause > 0 ? 'metric-val' : '',
    },
  ];
  const el = document.getElementById('overviewCards');
  el.innerHTML = cards
    .map(
      (cc) => `
    <div class="card">
      <div class="card-title">${cc.label}</div>
      <div class="metric-val ${cc.cls || ''}">${cc.val}</div>
      <div class="metric-sub">${cc.sub}</div>
    </div>
  `
    )
    .join('');

  const cpuSeries = genEMASeries(c.cpu.emaShort, Math.max(c.cpu.emaShort * 0.25, 1));
  const memSeries = genEMASeries(c.memory.emaShort, Math.max(c.memory.emaShort * 0.15, 0.1));
  const labels = timeLabels(20);
  const chartOpts = (unit, ySuffix) => ({
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: {
          label: (ctx) => ctx.dataset.label + ': ' + ctx.parsed.y.toFixed(1) + ySuffix,
        },
      },
    },
    scales: {
      x: {
        ticks: { color: '#52525b', font: { family: 'JetBrains Mono', size: 9 }, maxTicksLimit: 6 },
        grid: { color: '#1e1e21' },
      },
      y: {
        ticks: {
          color: '#52525b',
          font: { family: 'JetBrains Mono', size: 9 },
          callback: (v) => v + ySuffix,
        },
        grid: { color: '#1e1e21' },
      },
    },
    animation: { duration: 400 },
    elements: { point: { radius: 0 }, line: { tension: 0.3 } },
  });

  if (cpuChart) {
    cpuChart.destroy();
  }
  cpuChart = new Chart(document.getElementById('cpuChart'), {
    type: 'line',
    data: {
      labels,
      datasets: [
        { label: 'EMAShort', data: cpuSeries.short, borderColor: '#22d3ee', borderWidth: 1.5, fill: false },
        {
          label: 'EMALong',
          data: cpuSeries.long,
          borderColor: '#0891b2',
          borderWidth: 1,
          borderDash: [4, 3],
          fill: false,
        },
      ],
    },
    options: chartOpts('cpu', 'm'),
  });

  if (memChart) {
    memChart.destroy();
  }
  memChart = new Chart(document.getElementById('memChart'), {
    type: 'line',
    data: {
      labels,
      datasets: [
        { label: 'EMAShort', data: memSeries.short, borderColor: '#f97316', borderWidth: 1.5, fill: false },
        {
          label: 'EMALong',
          data: memSeries.long,
          borderColor: '#c2410c',
          borderWidth: 1,
          borderDash: [4, 3],
          fill: false,
        },
      ],
    },
    options: chartOpts('mem', 'Mi'),
  });
}

function fmtMi(v) {
  if (v >= 1024) return (v / 1024).toFixed(2) + 'Gi';
  return v + 'Mi';
}

function safeDiv(a, b) {
  const d = Math.max(b, 1e-9);
  return Math.min(100, (a / d) * 100);
}

function renderContainers() {
  const p = selectedProfile;
  const sel = document.getElementById('containerSel');
  sel.innerHTML = p.containers
    .map(
      (c, i) =>
        `<button class="csel-btn${i === selectedContainer ? ' active' : ''}" onclick="selectContainer(${i})">${c.name}</button>`
    )
    .join('');
  renderContainerDetail();
}

function selectContainer(i) {
  selectedContainer = i;
  document.querySelectorAll('.csel-btn').forEach((b, j) => b.classList.toggle('active', j === i));
  renderContainerDetail();
}

function renderContainerDetail() {
  const c = selectedProfile.containers[selectedContainer];
  const cpuArrow =
    c.cpu.emaShort > c.cpu.emaLong
      ? '<span class="ema-arrow arrow-up">↑</span>'
      : '<span class="ema-arrow arrow-down">↓</span>';
  const memArrow =
    c.memory.emaShort > c.memory.emaLong
      ? '<span class="ema-arrow arrow-up">↑</span>'
      : '<span class="ema-arrow arrow-down">↓</span>';
  const thr = c.throttleRatio;

  document.getElementById('containerDetail').innerHTML = `
    <div class="g2" style="margin-bottom:12px">
      <div class="card">
        <div class="card-title">CPU usage</div>
        <div style="display:flex;gap:20px;margin-bottom:10px">
          <div>
            <div style="font-size:10px;color:var(--txt3)">EMAShort ${cpuArrow}</div>
            <div style="font-size:20px;font-weight:600;color:var(--cpu)">${c.cpu.emaShort}m</div>
          </div>
          <div>
            <div style="font-size:10px;color:var(--txt3)">EMALong</div>
            <div style="font-size:20px;font-weight:600;color:var(--cpu-dim)">${c.cpu.emaLong}m</div>
          </div>
        </div>
        <div class="sig-row">
          <span class="sig-label">P50</span>
          <span class="sig-val metric-cpu">${c.cpu.p50.toFixed(0)}m</span>
          <div class="sig-bar-wrap"><div class="sig-bar bar-cpu" style="width:${safeDiv(c.cpu.p50, c.cpu.p90)}%"></div></div>
        </div>
        <div class="sig-row">
          <span class="sig-label">P75</span>
          <span class="sig-val metric-cpu">${c.cpu.p75.toFixed(0)}m</span>
          <div class="sig-bar-wrap"><div class="sig-bar bar-cpu" style="width:${safeDiv(c.cpu.p75, c.cpu.p90)}%"></div></div>
        </div>
        <div class="sig-row">
          <span class="sig-label">P90 (target)</span>
          <span class="sig-val metric-cpu">${c.cpu.p90.toFixed(0)}m</span>
          <div class="sig-bar-wrap"><div class="sig-bar bar-cpu" style="width:100%"></div></div>
        </div>
        <div class="sig-row">
          <span class="sig-label">Throttle ratio</span>
          <span class="sig-val ${thr != null && thr > 0.2 ? 'arrow-up' : ''}">${
            thr != null ? (thr * 100).toFixed(1) + '%' : '— (not on status)'
          }</span>
          ${
            thr != null
              ? `<div class="sig-bar-wrap"><div class="sig-bar ${
                  thr > 0.2 ? 'bar-err bar-warn' : 'bar-ok'
                }" style="width:${Math.min(100, thr * 100 * 3)}%"></div></div>`
              : ''
          }
        </div>
      </div>
      <div class="card">
        <div class="card-title">Memory usage ${
          c.memory.slopePositive ? '<span class="flag flag-err">trend_guard active</span>' : ''
        }</div>
        <div style="display:flex;gap:20px;margin-bottom:10px">
          <div>
            <div style="font-size:10px;color:var(--txt3)">EMAShort ${memArrow}</div>
            <div style="font-size:20px;font-weight:600;color:var(--mem)">${fmtMi(c.memory.emaShort)}</div>
          </div>
          <div>
            <div style="font-size:10px;color:var(--txt3)">EMALong</div>
            <div style="font-size:20px;font-weight:600;color:var(--mem-dim)">${fmtMi(c.memory.emaLong)}</div>
          </div>
        </div>
        <div class="sig-row">
          <span class="sig-label">P50</span>
          <span class="sig-val metric-mem">${fmtMi(c.memory.p50)}</span>
          <div class="sig-bar-wrap"><div class="sig-bar bar-mem" style="width:${safeDiv(c.memory.p50, c.memory.p90)}%"></div></div>
        </div>
        <div class="sig-row">
          <span class="sig-label">P90</span>
          <span class="sig-val metric-mem">${fmtMi(c.memory.p90)}</span>
          <div class="sig-bar-wrap"><div class="sig-bar bar-mem" style="width:100%"></div></div>
        </div>
        <div class="sig-row">
          <span class="sig-label">Slope streak</span>
          <span class="sig-val ${c.memory.slopePositive ? 'flag flag-err' : ''}">${c.memory.slopeStreak} / 5${
    c.memory.slopePositive ? ' (blocked)' : ''
  }</span>
        </div>
        <div class="streak-pips">
          ${Array.from(
            { length: 5 },
            (_, i) =>
              `<div class="pip ${
                i < c.memory.slopeStreak ? (c.memory.slopePositive ? 'pip-warn' : 'pip-filled') : ''
              }"></div>`
          ).join('')}
        </div>
      </div>
    </div>
    <div class="card">
      <div class="card-title">Current resources</div>
      <div style="display:flex;gap:24px;flex-wrap:wrap;font-size:12px">
        <div><span style="color:var(--txt3)">cpu request</span> <span style="color:var(--cpu)">${c.currentCPU.req}</span></div>
        <div><span style="color:var(--txt3)">cpu limit</span> <span style="color:var(--cpu)">${c.currentCPU.lim}</span></div>
        <div><span style="color:var(--txt3)">mem request</span> <span style="color:var(--mem)">${c.currentMem.req}</span></div>
        <div><span style="color:var(--txt3)">mem limit</span> <span style="color:var(--mem)">${c.currentMem.lim}</span></div>
        <div><span style="color:var(--txt3)">restarts</span> <span style="${
          c.restartCount > 3 ? 'color:var(--err)' : 'color:var(--txt)'
        }">${c.restartCount}</span></div>
        <div><span style="color:var(--txt3)">last OOM</span> <span style="${
          c.lastOOMKill ? 'color:var(--err)' : 'color:var(--txt3)'
        }">${c.lastOOMKill || 'none'}</span></div>
      </div>
    </div>
  `;
}

function parseRationale(rat) {
  if (!rat) return '';
  return rat
    .split(/[\s;]+/)
    .filter(Boolean)
    .map((t) => {
      let cls = '';
      if (t.startsWith('safety:')) cls = 'safety';
      else if (t === 'trend_guard') cls = 'trend';
      else if (t.includes('oom')) cls = 'oom';
      else if (t.startsWith('mode:')) cls = 'mode';
      return `<span class="rat-tag ${cls}">${t}</span>`;
    })
    .join('');
}

function valCmp(a, b) {
  const parse = (s) => {
    if (!s) return 0;
    const m = s.match(/^([\d.]+)(m|Mi|Gi)?$/);
    if (!m) return 0;
    let v = parseFloat(m[1]);
    if (m[2] === 'Gi') v *= 1024;
    return v;
  };
  const av = parse(a);
  const bv = parse(b);
  if (bv > av) return 'up';
  if (bv < av) return 'down';
  return 'same';
}

function renderRecommendations() {
  const p = selectedProfile;
  const tbody = document.getElementById('recoBody');
  document.getElementById('recoTs').textContent = p.lastEvaluated || '—';
  let rows = '';
  p.containers.forEach((c) => {
    const resources = [
      { r: 'CPU Request', m: c.metricsReco.cpuReq, s: c.safetyReco.cpuReq },
      { r: 'CPU Limit', m: c.metricsReco.cpuLim, s: c.safetyReco.cpuLim },
      { r: 'Memory Request', m: c.metricsReco.memReq, s: c.safetyReco.memReq },
      { r: 'Memory Limit', m: c.metricsReco.memLim, s: c.safetyReco.memLim },
    ];
    resources.forEach((row, i) => {
      const cmp = valCmp(row.m, row.s);
      const cmpClass = cmp === 'up' ? 'val-up' : cmp === 'down' ? 'val-down' : 'val-same';
      const delta = cmp === 'same' ? '—' : cmp === 'up' ? '▲ clamped' : '▼ stepped';
      rows += `<tr>
        ${i === 0 ? `<td class="container-col" rowspan="${resources.length}">${c.name}</td>` : ''}
        <td style="color:var(--txt3)">${row.r}</td>
        <td class="val-neutral">${row.m}</td>
        <td class="${cmpClass}">${row.s}</td>
        <td class="${cmpClass}" style="font-size:10px">${delta}</td>
        ${i === 0 ? `<td rowspan="${resources.length}"><div class="rationale">${parseRationale(c.rationale)}</div></td>` : ''}
      </tr>`;
    });
  });
  tbody.innerHTML = rows;
}

function renderSignals() {
  const p = selectedProfile;
  const c = p.containers[selectedContainer] || p.containers[0];

  const cusumEl = document.getElementById('cusumPanel');
  const cpuPct = Math.min(100, (c.cusumCPU.sPos / c.cusumCPU.h) * 100);
  const cpuNPct = Math.min(100, (c.cusumCPU.sNeg / c.cusumCPU.h) * 100);
  const memH = c.cusumMem.h;
  const memPct = Math.min(100, (c.cusumMem.sPos / memH) * 100);
  const memNPct = Math.min(100, (c.cusumMem.sNeg / memH) * 100);
  const threshPct = 100;

  cusumEl.innerHTML = `
    <div style="font-size:11px;color:var(--txt3);margin-bottom:10px">container: <span style="color:var(--txt2)">${c.name}</span> · threshold H shown in red</div>
    <div class="card-title">CPU · SPos</div>
    <div class="cusum-container">
      <div class="cusum-label-row"><span>0</span><span>H=${c.cusumCPU.h}</span></div>
      <div class="cusum-track">
        <div class="cusum-fill bar-cpu" style="width:${cpuPct}%"></div>
        <div class="cusum-threshold" style="left:${threshPct}%"><span class="cusum-threshold-label">H</span></div>
      </div>
      <div style="font-size:11px;color:var(--cpu);margin-top:4px">SPos = ${c.cusumCPU.sPos.toFixed(1)} ${
    c.cusumCPU.sPos > c.cusumCPU.h ? '<span class="flag flag-err">SHIFT</span>' : ''
  }</div>
    </div>
    <div class="card-title">CPU · SNeg</div>
    <div class="cusum-container">
      <div class="cusum-label-row"><span>0</span><span>H=${c.cusumCPU.h}</span></div>
      <div class="cusum-track">
        <div class="cusum-fill" style="width:${cpuNPct}%;background:var(--cpu-dim)"></div>
        <div class="cusum-threshold" style="left:${threshPct}%"><span class="cusum-threshold-label">H</span></div>
      </div>
      <div style="font-size:11px;color:var(--cpu-dim);margin-top:4px">SNeg = ${c.cusumCPU.sNeg.toFixed(1)}</div>
    </div>
    <div class="card-title" style="margin-top:10px">Memory · SPos</div>
    <div class="cusum-container">
      <div class="cusum-label-row"><span>0</span><span>H=${(memH / 1024 / 1024).toFixed(0)}MiB</span></div>
      <div class="cusum-track">
        <div class="cusum-fill bar-mem" style="width:${memPct}%"></div>
        <div class="cusum-threshold" style="left:${threshPct}%"><span class="cusum-threshold-label">H</span></div>
      </div>
      <div style="font-size:11px;color:var(--mem);margin-top:4px">SPos = ${(c.cusumMem.sPos / 1024 / 1024).toFixed(1)}MiB ${
    c.cusumMem.sPos > c.cusumMem.h ? '<span class="flag flag-err">SHIFT</span>' : ''
  }</div>
    </div>
    <div class="card-title">Memory · SNeg</div>
    <div class="cusum-container">
      <div class="cusum-label-row"><span>0</span><span>H=${(memH / 1024 / 1024).toFixed(0)}MiB</span></div>
      <div class="cusum-track">
        <div class="cusum-fill" style="width:${memNPct}%;background:var(--mem-dim)"></div>
        <div class="cusum-threshold" style="left:${threshPct}%"><span class="cusum-threshold-label">H</span></div>
      </div>
      <div style="font-size:11px;color:var(--mem-dim);margin-top:4px">SNeg = ${(c.cusumMem.sNeg / 1024 / 1024).toFixed(1)}MiB</div>
    </div>
  `;

  const slopeEl = document.getElementById('slopePanel');
  slopeEl.innerHTML = `
    <div style="font-size:11px;color:var(--txt3);margin-bottom:8px">streak resets on any non-increasing cycle · threshold = 5</div>
    <div style="font-size:12px;margin-bottom:8px">
      <span style="color:var(--txt2)">SlopeStreak:</span>
      <span style="color:${c.memory.slopePositive ? 'var(--err)' : 'var(--txt)'};font-weight:600"> ${c.memory.slopeStreak} / 5</span>
      ${
        c.memory.slopePositive
          ? '<span class="flag flag-err" style="margin-left:8px">memory downsize blocked</span>'
          : ''
      }
    </div>
    <div class="streak-pips">
      ${Array.from(
        { length: 5 },
        (_, i) =>
          `<div class="pip ${
            i < c.memory.slopeStreak ? (c.memory.slopePositive ? 'pip-warn' : 'pip-filled') : ''
          }"></div>`
      ).join('')}
    </div>
    <div style="font-size:10px;color:var(--txt3);margin-top:10px">Compares EMAShort(t) vs EMAShort(t-1) each reconcile. Positive streak gates memory downsize patches.</div>
  `;

  const thr = c.throttleRatio;
  const thrLabel =
    thr != null
      ? `${(thr * 100).toFixed(1)}%
        <span class="flag ${thr > 0.2 ? 'flag-err' : thr > 0.1 ? 'flag-warn' : 'flag-ok'}" style="margin-left:6px">${
            thr > 0.2 ? 'high' : thr > 0.1 ? 'moderate' : 'ok'
          }</span>`
      : '— (not persisted)';

  const podEl = document.getElementById('podPanel');
  podEl.innerHTML = `
    <div class="sig-row">
      <span class="sig-label">Restart count</span>
      <span class="sig-val ${c.restartCount > 3 ? 'flag flag-err' : ''}">${c.restartCount}</span>
    </div>
    <div class="sig-row">
      <span class="sig-label">Last OOM kill</span>
      <span class="sig-val ${c.lastOOMKill ? 'flag flag-err' : ''}">${c.lastOOMKill || 'none'}</span>
    </div>
    <div class="sig-row">
      <span class="sig-label">Throttle ratio</span>
      <span class="sig-val">${thrLabel}</span>
    </div>
    <div class="sig-row">
      <span class="sig-label">Downsize pause</span>
      <span class="sig-val ${p.downsizePause > 0 ? 'flag flag-warn' : ''}">${p.downsizePause} cycles remaining</span>
    </div>
  `;
}

function renderQuadrant() {
  const p = selectedProfile;
  const c = p.containers[selectedContainer] || p.containers[0];
  const quads = ['00:00–06:00', '06:00–12:00', '12:00–18:00', '18:00–24:00'];
  const qLabels = ['Q0 · night', 'Q1 · morning', 'Q2 · peak', 'Q3 · evening'];
  const el = document.getElementById('quadPanel');
  el.innerHTML = quads
    .map(
      (q, i) => `
    <div class="quad-card">
      <div class="quad-title">${q} <span style="color:var(--txt2);font-weight:500">${qLabels[i]}</span></div>
      <div class="quad-bar-label"><span style="color:var(--txt3)">CPU intensity</span><span style="color:var(--cpu)">${(c.quadCPU[i] * 100).toFixed(0)}%</span></div>
      <div class="quad-track"><div class="quad-fill" style="width:${c.quadCPU[i] * 100}%;background:var(--cpu)"></div></div>
      <div class="quad-bar-label"><span style="color:var(--txt3)">Memory intensity</span><span style="color:var(--mem)">${(c.quadMem[i] * 100).toFixed(0)}%</span></div>
      <div class="quad-track"><div class="quad-fill" style="width:${c.quadMem[i] * 100}%;background:var(--mem)"></div></div>
    </div>
  `
    )
    .join('');
}

function switchTab(id, el) {
  document.querySelectorAll('.tab').forEach((t) => t.classList.remove('active'));
  document.querySelectorAll('.panel').forEach((pn) => pn.classList.remove('active'));
  el.classList.add('active');
  document.getElementById('panel-' + id).classList.add('active');
}

loadProfiles();

setInterval(() => {
  if (selectedProfile && selectedProfile.lastEvaluated) {
    document.getElementById('topTs').textContent = 'evaluated ' + selectedProfile.lastEvaluated;
  }
}, 10000);
