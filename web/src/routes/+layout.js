// Single-user app: no authentication / session guard.
export const ssr = false;
export const prerender = false;

export async function load() {
  return {};
}
