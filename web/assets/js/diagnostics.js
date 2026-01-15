const refreshBtn = document.getElementById('refreshBtn');
const memorySummary = document.getElementById('memorySummary');
const servicesBody = document.getElementById('servicesBody');
const recommendations = document.getElementById('recommendations');
const recommendationsEmpty = document.getElementById('recommendationsEmpty');
const controlStatus = document.getElementById('controlStatus');

function formatBytes(bytes) {
    if (!bytes || bytes <= 0) return '0 B';
    const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    let value = bytes;
    let index = 0;
    while (value >= 1024 && index < units.length - 1) {
        value /= 1024;
        index += 1;
    }
    return `${value.toFixed(1)} ${units[index]}`;
}

function renderMemory(memory) {
    const cards = [
        { label: 'Total Memory', value: formatBytes(memory.totalBytes) },
        { label: 'Available Memory', value: formatBytes(memory.availableBytes) },
        { label: 'Swap Used', value: formatBytes(memory.swapUsedBytes) },
    ];
    memorySummary.innerHTML = cards.map((card) => `
        <div class="summary-card">
            <div class="label">${card.label}</div>
            <div class="value">${card.value}</div>
        </div>
    `).join('');
}

function statusClass(state) {
    if (!state) return '';
    if (state.toLowerCase() === 'running') {
        return 'running';
    }
    return 'exited';
}

function buildActionButton(service, state, enabled) {
    if (!enabled) {
        return '<button class="btn-secondary" disabled>Disabled</button>';
    }
    const isRunning = state.toLowerCase() === 'running';
    const action = isRunning ? 'stop' : 'start';
    const label = isRunning ? 'Stop' : 'Start';
    return `<button class="btn-secondary" data-service="${service}" data-action="${action}">${label}</button>`;
}

function renderServices(services, canControl) {
    servicesBody.innerHTML = services.map((service) => `
        <tr>
            <td>${service.service}</td>
            <td><span class="status-pill ${statusClass(service.state)}">${service.state || 'unknown'}</span></td>
            <td>${service.health || '-'}</td>
            <td>${formatBytes(service.memoryBytes)} (${service.memoryPct.toFixed(1)}%)</td>
            <td>${buildActionButton(service.service, service.state || '', canControl)}</td>
        </tr>
    `).join('');
}

function renderRecommendations(items) {
    if (!items || items.length === 0) {
        recommendations.innerHTML = '';
        recommendationsEmpty.textContent = 'No memory pressure detected.';
        return;
    }
    recommendationsEmpty.textContent = '';
    recommendations.innerHTML = items.map((item) => `
        <div class="recommendation">
            <h4>${item.service}</h4>
            <div>${item.reason}</div>
            <code>${item.command}</code>
        </div>
    `).join('');
}

async function fetchDiagnostics() {
    const response = await fetch('/api/diagnostics');
    if (!response.ok) {
        throw new Error(`Diagnostics failed (${response.status})`);
    }
    return response.json();
}

async function runAction(service, action) {
    const response = await fetch(`/api/diagnostics/services/${service}/${action}`, {
        method: 'POST'
    });
    if (!response.ok) {
        throw new Error(`Service action failed (${response.status})`);
    }
}

async function refresh() {
    try {
        const data = await fetchDiagnostics();
        renderMemory(data.memory);
        renderServices(data.containers, data.serviceControlEnabled);
        renderRecommendations(data.recommendations);
        controlStatus.textContent = data.serviceControlEnabled
            ? 'Service control enabled'
            : 'Service control disabled';
    } catch (error) {
        controlStatus.textContent = `${error}`;
    }
}

servicesBody.addEventListener('click', async (event) => {
    const button = event.target.closest('button');
    if (!button || button.disabled) {
        return;
    }
    const service = button.dataset.service;
    const action = button.dataset.action;
    if (!service || !action) {
        return;
    }
    button.disabled = true;
    try {
        await runAction(service, action);
        await refresh();
    } catch (error) {
        controlStatus.textContent = `${error}`;
    } finally {
        button.disabled = false;
    }
});

refreshBtn.addEventListener('click', refresh);

refresh();
