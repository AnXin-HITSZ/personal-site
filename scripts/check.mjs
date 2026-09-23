import { readdir } from 'node:fs/promises';
import { spawnSync } from 'node:child_process';
for (const dir of ['frontend/src', 'scripts', 'tests']) {
  for (const file of await readdir(dir, { recursive: true })) {
    if (!/\.m?js$/.test(file)) continue;
    const result = spawnSync(process.execPath, ['--check', `${dir}/${file}`], { stdio: 'inherit' });
    if (result.status !== 0) process.exit(result.status || 1);
  }
}
console.log('JavaScript syntax checks passed.');
