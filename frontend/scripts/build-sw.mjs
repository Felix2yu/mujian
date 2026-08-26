import { readFileSync, writeFileSync } from 'node:fs';
import { execSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..');

// 用 git sha 作为构建版本号（CI / 本地均可用）；取不到时退化为时间戳，保证每次构建字节不同
let version = '';
try {
  version = execSync('git rev-parse --short HEAD', { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
} catch { /* 非 git 环境 */ }
if (!version) version = 'dev-' + Date.now().toString(36);

const tpl = readFileSync(join(here, 'sw.template.js'), 'utf8');
const out = tpl.replaceAll('__BUILD_VERSION__', version);
writeFileSync(join(root, 'static', 'sw.js'), out);
console.log('[build-sw] generated static/sw.js with version', version);
