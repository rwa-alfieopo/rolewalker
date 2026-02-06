const API_BASE = '/api';

function showAddForm(type) {
    document.getElementById('addFormContainer').style.display = 'block';
    document.getElementById('accountForm').style.display = type === 'account' ? 'block' : 'none';
    document.getElementById('roleForm').style.display = type === 'role' ? 'block' : 'none';
}

function hideAddForm() {
    document.getElementById('addFormContainer').style.display = 'none';
    document.getElementById('addAccountForm').reset();
    document.getElementById('addRoleForm').reset();
}

async function handleAddAccount(event) {
    event.preventDefault();
    const data = {
        account_id: document.getElementById('accountId').value,
        account_name: document.getElementById('accountName').value,
        sso_start_url: document.getElementById('ssoStartUrl').value,
        sso_region: document.getElementById('ssoRegion').value,
        description: document.getElementById('accountDesc').value
    };

    try {
        const response = await fetch(`${API_BASE}/accounts`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        });

        if (response.ok) {
            showMessage('Account added successfully', 'success', 'quickAddMessage');
            hideAddForm();
            await loadAccounts();
        } else {
            const error = await response.json();
            showMessage(error.error || 'Failed to add account', 'error', 'quickAddMessage');
        }
    } catch (error) {
        showMessage('Error: ' + error.message, 'error', 'quickAddMessage');
    }
}

async function handleAddRole(event) {
    event.preventDefault();
    const data = {
        account_id: parseInt(document.getElementById('roleAccountId').value),
        role_name: document.getElementById('roleName').value,
        role_arn: document.getElementById('roleArn').value,
        profile_name: document.getElementById('profileName').value,
        region: document.getElementById('roleRegion').value,
        description: document.getElementById('roleDesc').value
    };

    try {
        const response = await fetch(`${API_BASE}/roles`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        });

        if (response.ok) {
            showMessage('Role added successfully', 'success', 'quickAddMessage');
            hideAddForm();
            await loadAccounts();
        } else {
            const error = await response.json();
            showMessage(error.error || 'Failed to add role', 'error', 'quickAddMessage');
        }
    } catch (error) {
        showMessage('Error: ' + error.message, 'error', 'quickAddMessage');
    }
}

async function loadAccounts() {
    try {
        const response = await fetch(`${API_BASE}/accounts`);
        const accounts = await response.json();
        renderAccounts(accounts);
        updateRoleAccountSelect(accounts);
    } catch (error) {
        console.error('Failed to load accounts:', error);
    }
}

async function loadActiveSession() {
    try {
        const response = await fetch(`${API_BASE}/session/active`);
        const session = await response.json();
        renderSession(session);
    } catch (error) {
        console.error('Failed to load session:', error);
    }
}

function renderSession(session) {
    const info = document.getElementById('sessionInfo');
    if (!session.active || !session.role || !session.account) {
        info.innerHTML = '<p>No active session</p>';
        return;
    }

    info.innerHTML = `
        <div class="session-columns">
            <div class="session-column aws-column">
                <div><strong>AWS Profile:</strong>     ${session.role.profile_name}</div>
                <div><strong>AWS Region:</strong>      ${session.role.region}</div>
                <div><strong>Account ID:</strong>      ${session.account.account_id}</div>
                <div><strong>Account Name:</strong>    ${session.account.account_name}</div>
                <div><strong>Role:</strong>            ${session.role.role_name}</div>
            </div>
            <div class="session-column kube-column">
                <div><strong>Kube Cluster:</strong>    ${session.kube_context || 'N/A'}</div>
                <div><strong>Kube Namespace:</strong>  ${session.kube_namespace || 'default'}</div>
            </div>
        </div>
    `;
}

function renderAccounts(accounts) {
    const list = document.getElementById('accountsList');
    if (accounts.length === 0) {
        list.innerHTML = '<div class="empty-state"><p>No accounts configured</p></div>';
        return;
    }

    list.innerHTML = accounts.map(account => `
        <div class="account-item" onclick="toggleAccount(this, ${account.id})">
            <div class="account-name">${account.account_name}</div>
            <div class="account-id">${account.account_id}</div>
            <div class="roles-list" id="roles-${account.id}"></div>
        </div>
    `).join('');
}

async function toggleAccount(element, accountId) {
    element.classList.toggle('expanded');
    if (element.classList.contains('expanded')) {
        await loadRoles(accountId);
    }
}

async function loadRoles(accountId) {
    try {
        const response = await fetch(`${API_BASE}/accounts/${accountId}/roles`);
        const roles = await response.json();
        renderRoles(accountId, roles);
    } catch (error) {
        console.error('Failed to load roles:', error);
    }
}

function renderRoles(accountId, roles) {
    const container = document.getElementById(`roles-${accountId}`);
    if (!roles || roles.length === 0) {
        container.innerHTML = '<div style="padding: 8px; color: #999;">No roles</div>';
        return;
    }

    container.innerHTML = roles.map(role => `
        <div class="role-item">
            <div class="role-info">
                <div class="role-name">${role.role_name}</div>
                <div class="role-profile">${role.profile_name} • ${role.region}</div>
            </div>
            <div class="role-actions">
                <button class="role-login-btn login-expired" id="login-${role.profile_name}" onclick="loginRole('${role.profile_name}', event)">Login</button>
                <button class="role-switch-btn" onclick="switchRole('${role.profile_name}', event)">Switch</button>
            </div>
        </div>
    `).join('');
    
    // Check login status for each role
    roles.forEach(role => {
        checkLoginStatus(role.profile_name);
    });
}

async function checkLoginStatus(profileName) {
    try {
        const response = await fetch(`${API_BASE}/session/login-status/${profileName}`);
        const data = await response.json();
        
        const button = document.getElementById(`login-${profileName}`);
        if (!button) return;
        
        if (data.logged_in) {
            button.textContent = 'Logged In';
            button.classList.remove('login-expired');
            button.classList.add('login-active');
        } else {
            button.textContent = 'Login';
            button.classList.remove('login-active');
            button.classList.add('login-expired');
        }
    } catch (error) {
        console.error('Failed to check login status:', error);
    }
}

async function switchRole(profileName, event) {
    event.stopPropagation();
    
    // Check if this is a production role
    const isProd = profileName.toLowerCase().includes('prod') || 
                   profileName.toLowerCase().includes('live') ||
                   profileName.toLowerCase().includes('production');
    
    // Store the profile name for confirmation
    window.pendingProfileSwitch = profileName;
    
    // Show confirmation modal
    const confirmMsg = document.getElementById('confirmMessage');
    confirmMsg.textContent = `Switch to role: ${profileName}?`;
    
    const warningBox = document.getElementById('warningBox');
    warningBox.style.display = isProd ? 'block' : 'none';
    
    document.getElementById('confirmModal').classList.add('show');
}

async function loginRole(profileName, event) {
    event.stopPropagation();
    event.preventDefault();
    
    const button = document.getElementById(`login-${profileName}`);
    if (button && button.classList.contains('login-active')) {
        console.log('Button is in login-active state, skipping');
        return; // Already logged in, don't proceed
    }
    
    console.log('Starting login for profile:', profileName);
    
    try {
        showMessage('Starting SSO login...', 'success');
        const response = await fetch(`${API_BASE}/session/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ profile_name: profileName })
        });

        console.log('Login response status:', response.status);

        if (response.ok) {
            showMessage('SSO login successful and role switched', 'success');
            await loadActiveSession();
            // Refresh login status
            checkLoginStatus(profileName);
        } else {
            const error = await response.json();
            console.error('Login error:', error);
            showMessage(error.error || 'Failed to login', 'error');
        }
    } catch (error) {
        console.error('Login exception:', error);
        showMessage('Failed to login: ' + error.message, 'error');
    }
}

async function confirmSwitch() {
    const profileName = window.pendingProfileSwitch;
    
    try {
        const response = await fetch(`${API_BASE}/session/switch`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ profile_name: profileName })
        });

        if (response.ok) {
            showMessage('Session switched successfully', 'success');
            closeConfirmModal();
            await loadActiveSession();
        } else {
            const error = await response.json();
            showMessage(error.error || 'Failed to switch session', 'error');
        }
    } catch (error) {
        showMessage('Failed to switch session: ' + error.message, 'error');
    }
}

function closeConfirmModal() {
    document.getElementById('confirmModal').classList.remove('show');
    window.pendingProfileSwitch = null;
}

function updateRoleAccountSelect(accounts) {
    const select = document.getElementById('roleAccountId');
    const currentValue = select.value;
    select.innerHTML = '<option value="">Select an account</option>' +
        accounts.map(acc => `<option value="${acc.id}">${acc.account_name} (${acc.account_id})</option>`).join('');
    select.value = currentValue;
}

async function importConfig() {
    try {
        const response = await fetch(`${API_BASE}/config/import`);
        const data = await response.json();
        
        window.importProfiles = data.profiles;
        const content = document.getElementById('modalPreviewContent');
        content.textContent = JSON.stringify(data.profiles, null, 2);
        
        document.getElementById('importModal').classList.add('show');
        showMessage('Profiles loaded. Click "Save All Profiles" to import them.', 'success', 'importMessage');
    } catch (error) {
        showMessage('Failed to import config: ' + error.message, 'error', 'importMessage');
    }
}

function closeImportModal() {
    document.getElementById('importModal').classList.remove('show');
}

async function saveImportedProfiles(profiles) {
    try {
        const response = await fetch(`${API_BASE}/config/import`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ profiles })
        });

        const result = await response.json();
        
        if (response.ok) {
            let msg = `Successfully imported ${result.imported} profiles`;
            if (result.skipped > 0) {
                msg += ` (${result.skipped} skipped as duplicates)`;
            }
            showMessage(msg, 'success', 'importMessage');
            
            if (result.errors && result.errors.length > 0) {
                console.log('Import errors:', result.errors);
                result.errors.forEach(err => {
                    showMessage(`Warning: ${err}`, 'error', 'importMessage');
                });
            }
            closeImportModal();
            await loadAccounts();
        } else {
            showMessage(result.error || 'Failed to save profiles', 'error', 'importMessage');
        }
    } catch (error) {
        showMessage('Error: ' + error.message, 'error', 'importMessage');
    }
}

function showMessage(text, type, elementId = null) {
    const messageEl = elementId ? document.getElementById(elementId) : document.createElement('div');
    messageEl.className = `message ${type} show`;
    messageEl.textContent = text;
    
    if (!elementId) {
        document.body.insertBefore(messageEl, document.body.firstChild);
        setTimeout(() => messageEl.remove(), 5000);
    } else {
        setTimeout(() => messageEl.classList.remove('show'), 5000);
    }
}

// Load data on page load
loadAccounts();
loadActiveSession();

// Refresh session every 5 seconds
setInterval(loadActiveSession, 5000);

// Refresh login status for all visible roles every 30 seconds
setInterval(() => {
    document.querySelectorAll('[id^="login-"]').forEach(button => {
        const profileName = button.id.replace('login-', '');
        checkLoginStatus(profileName);
    });
}, 30000);
