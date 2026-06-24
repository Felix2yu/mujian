<script>
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';

  let { data } = $props();
  let user = $state(data?.user);

  let oldPassword = $state('');
  let newPassword = $state('');
  let confirmPassword = $state('');
  let passwordError = $state('');
  let passwordSuccess = $state('');
  let passwordLoading = $state(false);

  let showDeleteConfirm = $state(false);
  let deleteLoading = $state(false);

  async function handleChangePassword(e) {
    e.preventDefault();
    passwordError = '';
    passwordSuccess = '';

    if (newPassword !== confirmPassword) {
      passwordError = '两次输入的新密码不一致';
      return;
    }

    if (newPassword.length < 6) {
      passwordError = '新密码至少需要6个字符';
      return;
    }

    passwordLoading = true;
    try {
      await api.changePassword(oldPassword, newPassword);
      passwordSuccess = '密码修改成功';
      oldPassword = '';
      newPassword = '';
      confirmPassword = '';
    } catch (e) {
      passwordError = e.message || '修改失败';
    } finally {
      passwordLoading = false;
    }
  }

  async function handleDeleteAccount() {
    deleteLoading = true;
    try {
      await api.deleteAccount();
      goto('/login');
    } catch (e) {
      alert('删除失败: ' + e.message);
    } finally {
      deleteLoading = false;
      showDeleteConfirm = false;
    }
  }
</script>

<div class="settings-page">
  <div class="page-header">
    <h1>账户设置</h1>
  </div>

  <div class="settings-card">
    <div class="section">
      <h2>账户信息</h2>
      <div class="info-row">
        <span class="info-label">用户名</span>
        <span class="info-value">{user?.username || '-'}</span>
      </div>
    </div>

    <div class="section">
      <h2>修改密码</h2>

      {#if passwordError}
        <div class="error">{passwordError}</div>
      {/if}

      {#if passwordSuccess}
        <div class="success">{passwordSuccess}</div>
      {/if}

      <form onsubmit={handleChangePassword} class="form">
        <div class="form-group">
          <label for="old-password">当前密码</label>
          <input id="old-password" type="password" bind:value={oldPassword} placeholder="输入当前密码" required />
        </div>
        <div class="form-group">
          <label for="new-password">新密码</label>
          <input id="new-password" type="password" bind:value={newPassword} placeholder="至少6个字符" required />
        </div>
        <div class="form-group">
          <label for="confirm-password">确认新密码</label>
          <input id="confirm-password" type="password" bind:value={confirmPassword} placeholder="再次输入新密码" required />
        </div>
        <button type="submit" class="submit-btn" disabled={passwordLoading}>
          {passwordLoading ? '保存中...' : '修改密码'}
        </button>
      </form>
    </div>

    <div class="section danger-section">
      <h2>危险操作</h2>
      <p class="danger-desc">删除账户将永久清除所有数据，此操作不可恢复。</p>
      {#if showDeleteConfirm}
        <div class="confirm-box">
          <p>确定要删除账户吗？所有演出数据将被永久删除。</p>
          <div class="confirm-actions">
            <button class="cancel-btn" onclick={() => showDeleteConfirm = false}>取消</button>
            <button class="delete-btn" onclick={handleDeleteAccount} disabled={deleteLoading}>
              {deleteLoading ? '删除中...' : '确认删除'}
            </button>
          </div>
        </div>
      {:else}
        <button class="delete-account-btn" onclick={() => showDeleteConfirm = true}>删除账户</button>
      {/if}
    </div>
  </div>
</div>

<style>
  .settings-page { max-width: 600px; margin: 0 auto; }

  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 28px;
    flex-wrap: wrap;
    gap: 12px;
  }

  h1 { font-size: 24px; font-weight: 700; letter-spacing: -0.02em; }

  .settings-card {
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
    overflow: hidden;
  }

  .section {
    padding: 28px 32px;
    border-bottom: 1px solid var(--border);
  }

  .section:last-child {
    border-bottom: none;
  }

  .section h2 {
    font-size: 16px;
    font-weight: 600;
    margin-bottom: 16px;
    color: var(--text-primary);
  }

  .info-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 12px 0;
  }

  .info-label {
    font-size: 14px;
    color: var(--text-secondary);
  }

  .info-value {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-primary);
  }

  .error {
    background: var(--danger-bg);
    color: var(--danger-text);
    padding: 12px 16px;
    border-radius: var(--radius-sm);
    margin-bottom: 16px;
    font-size: 14px;
    font-weight: 500;
  }

  .success {
    background: var(--success-bg);
    color: var(--success);
    padding: 12px 16px;
    border-radius: var(--radius-sm);
    margin-bottom: 16px;
    font-size: 14px;
    font-weight: 500;
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .form-group label {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-secondary);
  }

  .form-group input {
    width: 100%;
    padding: 12px 14px;
    border-radius: var(--radius-sm);
    font-size: 14px;
    border: 1.5px solid var(--border);
    background: var(--bg-input);
    color: var(--text-primary);
    transition: all 0.2s;
  }

  .form-group input:hover {
    border-color: var(--border-hover);
  }

  .form-group input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
    outline: none;
  }

  .submit-btn {
    width: 100%;
    padding: 12px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-sm);
    font-size: 14px;
    font-weight: 600;
    transition: all 0.2s;
    margin-top: 8px;
  }

  .submit-btn:hover:not(:disabled) {
    background: var(--accent-light);
    transform: translateY(-1px);
  }

  .submit-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .danger-section {
    background: var(--danger-bg);
  }

  .danger-section h2 {
    color: var(--danger-text);
  }

  .danger-desc {
    font-size: 14px;
    color: var(--text-secondary);
    margin-bottom: 16px;
    line-height: 1.5;
  }

  .confirm-box {
    background: var(--bg-card);
    border-radius: var(--radius-sm);
    padding: 16px;
    border: 1px solid var(--border);
  }

  .confirm-box p {
    font-size: 14px;
    color: var(--text-secondary);
    margin-bottom: 12px;
  }

  .confirm-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .cancel-btn {
    padding: 8px 16px;
    border-radius: var(--radius-sm);
    font-size: 13px;
    font-weight: 500;
    background: var(--bg-surface);
    color: var(--text-secondary);
    border: 1px solid var(--border);
    transition: all 0.15s;
  }

  .cancel-btn:hover {
    background: var(--bg-surface-hover);
  }

  .delete-btn {
    padding: 8px 16px;
    border-radius: var(--radius-sm);
    font-size: 13px;
    font-weight: 500;
    background: var(--danger-text);
    color: #fff;
    transition: all 0.15s;
  }

  .delete-btn:hover:not(:disabled) {
    opacity: 0.9;
  }

  .delete-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .delete-account-btn {
    padding: 10px 20px;
    border-radius: var(--radius-sm);
    font-size: 14px;
    font-weight: 500;
    background: var(--danger-text);
    color: #fff;
    transition: all 0.15s;
  }

  .delete-account-btn:hover {
    opacity: 0.9;
  }

  @media (max-width: 480px) {
    .section { padding: 20px 16px; }
  }
</style>
