import { redirect } from '@sveltejs/kit';

export async function load({ url, fetch }) {
  const publicPaths = ['/login', '/register'];
  const isPublicPath = publicPaths.includes(url.pathname);

  try {
    const res = await fetch('/api/auth/me', { credentials: 'include' });
    if (res.ok) {
      const user = await res.json();
      return { user };
    }
  } catch {}

  if (!isPublicPath) {
    throw redirect(302, '/login');
  }

  return { user: null };
}
