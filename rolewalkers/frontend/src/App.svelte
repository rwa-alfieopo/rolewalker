<script lang="ts">
  import { onMount } from 'svelte';
  import { AWSService } from "../bindings/rolewalkers/services";

  interface Profile {
    name: string;
    region: string;
    ssoStartUrl: string;
    ssoRegion: string;
    ssoAccountId: string;
    ssoRoleName: string;
    isSso: boolean;
    isActive: boolean;
    isLoggedIn: boolean;
  }

  let profiles: Profile[] = [];
  let loading = true;
  let error = '';
  let activeProfile = '';
  let searchTerm = '';
  let showSSOOnly = false;
  let actionInProgress = '';

  $: filteredProfiles = profiles.filter(p => {
    // Filter out sso-session entries (they're not real profiles)
    if (p.name.startsWith('sso-session')) return false;
    
    const matchesSearch = p.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      p.ssoAccountId?.includes(searchTerm) ||
      p.ssoRoleName?.toLowerCase().includes(searchTerm.toLowerCase());
    const matchesFilter = !showSSOOnly || p.isSso;
    return matchesSearch && matchesFilter;
  });

  onMount(async () => {
    await loadProfiles();
  });

  async function loadProfiles() {
    loading = true;
    error = '';
    try {
      profiles = await AWSService.GetProfiles();
      activeProfile = await AWSService.GetActiveProfile();
    } catch (e: any) {
      error = e.message || 'Failed to load profiles';
    } finally {
      loading = false;
    }
  }

  async function switchProfile(profileName: string) {
    actionInProgress = profileName;
    try {
      await AWSService.SwitchProfile(profileName);
      await loadProfiles();
    } catch (e: any) {
      error = e.message || 'Failed to switch profile';
    } finally {
      actionInProgress = '';
    }
  }

  async function login(profileName: string) {
    actionInProgress = profileName;
    try {
      await AWSService.Login(profileName);
      await loadProfiles();
    } catch (e: any) {
      error = e.message || 'Login failed';
    } finally {
      actionInProgress = '';
    }
  }

  async function logout(profileName: string) {
    actionInProgress = profileName;
    try {
      await AWSService.Logout(profileName);
      await loadProfiles();
    } catch (e: any) {
      error = e.message || 'Logout failed';
    } finally {
      actionInProgress = '';
    }
  }

  async function loginAndSwitch(profileName: string) {
    actionInProgress = profileName;
    try {
      await AWSService.LoginAndSwitch(profileName);
      await loadProfiles();
    } catch (e: any) {
      error = e.message || 'Login and switch failed';
    } finally {
      actionInProgress = '';
    }
  }
</script>

<main>
  <header>
    <h1>🔐 rolewalkers</h1>
    <p class="subtitle">AWS Profile & SSO Manager</p>
  </header>

  {#if error}
    <div class="error-banner">
      <span>{error}</span>
      <button on:click={() => error = ''}>✕</button>
    </div>
  {/if}

  <div class="active-profile">
    <span class="label">Active Profile:</span>
    <span class="value">{activeProfile || 'None'}</span>
    <button class="refresh-btn" on:click={loadProfiles} disabled={loading}>
      {loading ? '⟳' : '↻'} Refresh
    </button>
  </div>

  <div class="controls">
    <input
      type="text"
      placeholder="Search profiles..."
      bind:value={searchTerm}
      class="search-input"
    />
    <label class="filter-toggle">
      <input type="checkbox" bind:checked={showSSOOnly} />
      SSO Only
    </label>
  </div>

  {#if loading}
    <div class="loading">Loading profiles...</div>
  {:else if filteredProfiles.length === 0}
    <div class="empty">
      {#if profiles.length === 0}
        No AWS profiles found. Configure profiles in ~/.aws/config
      {:else}
        No profiles match your search.
      {/if}
    </div>
  {:else}
    <div class="profiles-list">
      {#each filteredProfiles as profile}
        <div class="profile-card" class:active={profile.isActive}>
          <div class="profile-main">
            <div class="profile-info">
              <div class="profile-name">
                <h3>{profile.name}</h3>
                <div class="badges">
                  {#if profile.isActive}
                    <span class="badge active-badge">ACTIVE</span>
                  {/if}
                  {#if profile.isSso}
                    <span class="badge sso-badge" class:logged-in={profile.isLoggedIn}>
                      {profile.isLoggedIn ? '✓ SSO' : '○ SSO'}
                    </span>
                  {/if}
                </div>
              </div>
              <div class="profile-meta">
                {#if profile.region}
                  <span class="meta-item">📍 {profile.region}</span>
                {/if}
                {#if profile.ssoAccountId}
                  <span class="meta-item">🏢 {profile.ssoAccountId}</span>
                {/if}
                {#if profile.ssoRoleName}
                  <span class="meta-item">👤 {profile.ssoRoleName}</span>
                {/if}
              </div>
            </div>

            <div class="profile-actions">
              {#if !profile.isActive}
                <button
                  class="btn btn-primary"
                  on:click={() => switchProfile(profile.name)}
                  disabled={actionInProgress === profile.name}
                >
                  {actionInProgress === profile.name ? '...' : 'Switch'}
                </button>
              {/if}

              {#if profile.isSso}
                {#if profile.isLoggedIn}
                  <button
                    class="btn btn-secondary"
                    on:click={() => logout(profile.name)}
                    disabled={actionInProgress === profile.name}
                  >
                    Logout
                  </button>
                {:else}
                  <button
                    class="btn btn-success"
                    on:click={() => login(profile.name)}
                    disabled={actionInProgress === profile.name}
                  >
                    Login
                  </button>
                  {#if !profile.isActive}
                    <button
                      class="btn btn-accent"
                      on:click={() => loginAndSwitch(profile.name)}
                      disabled={actionInProgress === profile.name}
                      title="Login and Switch"
                    >
                      Login + Switch
                    </button>
                  {/if}
                {/if}
              {/if}
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <footer>
    <p>CLI: <code>rwcli help</code></p>
  </footer>
</main>

<style>
  :global(body) {
    margin: 0;
    padding: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
    background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
    color: #e4e4e4;
    min-height: 100vh;
  }

  main {
    max-width: 800px;
    margin: 0 auto;
    padding: 20px;
  }

  header {
    text-align: center;
    margin-bottom: 24px;
    padding-top: 16px;
  }

  h1 {
    margin: 0;
    font-size: 2rem;
    background: linear-gradient(90deg, #ff9a56, #ff6b6b);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
  }

  .subtitle {
    color: #888;
    margin-top: 4px;
    font-size: 0.9rem;
  }

  .error-banner {
    background: #ff4757;
    color: white;
    padding: 10px 16px;
    border-radius: 8px;
    margin-bottom: 16px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 0.9rem;
  }

  .error-banner button {
    background: none;
    border: none;
    color: white;
    cursor: pointer;
    font-size: 1rem;
    padding: 0 4px;
  }

  .active-profile {
    background: rgba(255, 255, 255, 0.05);
    padding: 12px 16px;
    border-radius: 8px;
    margin-bottom: 16px;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .active-profile .label {
    color: #888;
    font-size: 0.9rem;
  }

  .active-profile .value {
    font-weight: 600;
    color: #ff9a56;
    flex: 1;
  }

  .refresh-btn {
    background: rgba(255, 255, 255, 0.1);
    border: none;
    color: #e4e4e4;
    padding: 6px 12px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.85rem;
    transition: background 0.2s;
  }

  .refresh-btn:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.2);
  }

  .controls {
    display: flex;
    gap: 12px;
    margin-bottom: 16px;
    align-items: center;
  }

  .search-input {
    flex: 1;
    padding: 10px 14px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.05);
    color: #e4e4e4;
    font-size: 0.9rem;
  }

  .search-input:focus {
    outline: none;
    border-color: #ff9a56;
  }

  .filter-toggle {
    display: flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    color: #888;
    font-size: 0.85rem;
    white-space: nowrap;
  }

  .loading, .empty {
    text-align: center;
    padding: 40px;
    color: #888;
  }

  .profiles-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .profile-card {
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 10px;
    padding: 14px 16px;
    transition: background 0.2s;
  }

  .profile-card:hover {
    background: rgba(255, 255, 255, 0.06);
  }

  .profile-card.active {
    border-color: #ff9a56;
    background: rgba(255, 154, 86, 0.08);
  }

  .profile-main {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
  }

  .profile-info {
    flex: 1;
    min-width: 0;
  }

  .profile-name {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 6px;
    flex-wrap: wrap;
  }

  .profile-name h3 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
  }

  .badges {
    display: flex;
    gap: 6px;
  }

  .badge {
    font-size: 0.65rem;
    padding: 3px 6px;
    border-radius: 4px;
    font-weight: 600;
    text-transform: uppercase;
  }

  .active-badge {
    background: #ff9a56;
    color: #1a1a2e;
  }

  .sso-badge {
    background: rgba(255, 255, 255, 0.1);
    color: #888;
  }

  .sso-badge.logged-in {
    background: #2ed573;
    color: #1a1a2e;
  }

  .profile-meta {
    display: flex;
    gap: 16px;
    flex-wrap: wrap;
  }

  .meta-item {
    font-size: 0.8rem;
    color: #888;
  }

  .profile-actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }

  .btn {
    padding: 8px 14px;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.8rem;
    font-weight: 500;
    transition: opacity 0.2s, transform 0.1s;
    white-space: nowrap;
  }

  .btn:hover:not(:disabled) {
    opacity: 0.9;
  }

  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-primary {
    background: #ff9a56;
    color: #1a1a2e;
  }

  .btn-secondary {
    background: rgba(255, 255, 255, 0.1);
    color: #e4e4e4;
  }

  .btn-success {
    background: #2ed573;
    color: #1a1a2e;
  }

  .btn-accent {
    background: #5352ed;
    color: white;
  }

  footer {
    text-align: center;
    margin-top: 32px;
    padding: 16px;
    color: #555;
    font-size: 0.85rem;
  }

  footer code {
    background: rgba(255, 255, 255, 0.08);
    padding: 3px 6px;
    border-radius: 4px;
    font-family: 'Fira Code', 'Consolas', monospace;
  }
</style>
