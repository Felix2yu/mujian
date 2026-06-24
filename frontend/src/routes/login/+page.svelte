<script>
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';

  let username = $state('');
  let password = $state('');
  let error = $state('');
  let loading = $state(false);

  async function handleSubmit(e) {
    e.preventDefault();
    error = '';
    loading = true;
    try {
      await api.login(username, password);
      goto('/');
    } catch (e) {
      error = e.message || '登录失败';
    } finally {
      loading = false;
    }
  }
</script>

<div class="auth-page">
  <div class="auth-card">
    <div class="auth-header">
      <h1>幕间</h1>
      <p>登录你的账号</p>
    </div>

    {#if error}
      <div class="error">{error}</div>
    {/if}

    <form onsubmit={handleSubmit} class="auth-form">
      <div class="form-group">
        <label for="username">用户名</label>
        <input id="username" type="text" bind:value={username} placeholder="输入用户名" required autofocus />
      </div>
      <div class="form-group">
        <label for="password">密码</label>
        <input id="password" type="password" bind:value={password} placeholder="输入密码" required />
      </div>
      <button type="submit" class="submit-btn" disabled={loading}>
        {loading ? '登录中...' : '登录'}
      </button>
    </form>

    <div class="auth-footer">
      <p>还没有账号？<a href="/register">立即注册</a></p>
    </div>
  </div>
</div>

<style>
  .auth-page {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
    background: var(--bg-body);
  }

  .auth-card {
    width: 100%;
    max-width: 400px;
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    padding: 40px;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-md);
  }

  .auth-header {
    text-align: center;
    margin-bottom: 32px;
  }

  .auth-header h1 {
    font-size: 28px;
    font-weight: 800;
    background: linear-gradient(135deg, #6366f1, #8b5cf6);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
    margin-bottom: 8px;
  }

  .auth-header p {
    font-size: 14px;
    color: var(--text-muted);
  }

  .error {
    background: var(--danger-bg);
    color: var(--danger-text);
    padding: 12px 16px;
    border-radius: var(--radius-sm);
    margin-bottom: 20px;
    font-size: 14px;
    font-weight: 500;
  }

  .auth-form {
    display: flex;
    flex-direction: column;
    gap: 20px;
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
    padding: 12px 16px;
    border-radius: var(--radius-sm);
    font-size: 15px;
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
    padding: 14px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-sm);
    font-size: 15px;
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

  .auth-footer {
    text-align: center;
    margin-top: 24px;
    font-size: 14px;
    color: var(--text-muted);
  }

  .auth-footer a {
    color: var(--accent);
    text-decoration: none;
    font-weight: 500;
  }

  .auth-footer a:hover {
    text-decoration: underline;
  }
</style>
