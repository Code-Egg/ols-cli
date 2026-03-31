const { createHash } = require('node:crypto');
const { createWriteStream, existsSync, mkdirSync, readFileSync } = require('node:fs');
const { chmodSync, renameSync } = require('node:fs');
const { get } = require('node:https');
const { join } = require('node:path');

const REPO = 'eric7/ols-cli';
const VERSION = process.env.npm_package_version || '0.1.0-dev';

function getArch() {
  switch (process.arch) {
    case 'x64':
      return 'amd64';
    case 'arm64':
      return 'arm64';
    default:
      throw new Error(`unsupported architecture: ${process.arch}`);
  }
}

function download(url, destination) {
  return new Promise((resolve, reject) => {
    const file = createWriteStream(destination);
    get(url, (response) => {
      if (response.statusCode !== 200) {
        reject(new Error(`download failed (${response.statusCode}) for ${url}`));
        return;
      }
      response.pipe(file);
      file.on('finish', () => {
        file.close(resolve);
      });
    }).on('error', reject);
  });
}

function sha256(path) {
  const hash = createHash('sha256');
  hash.update(readFileSync(path));
  return hash.digest('hex');
}

function checksumFor(fileName, checksumsText) {
  const line = checksumsText
    .split('\n')
    .map((v) => v.trim())
    .find((v) => v.endsWith(` ${fileName}`) || v.endsWith(`  ${fileName}`));

  if (!line) {
    throw new Error(`checksum entry missing for ${fileName}`);
  }

  return line.split(/\s+/)[0];
}

async function run() {
  if (process.platform !== 'linux') {
    console.log('[ols npm] non-linux platform detected, skipping binary download');
    return;
  }

  const arch = getArch();
  const tag = VERSION.includes('dev') ? 'latest' : `v${VERSION.replace(/^v/, '')}`;
  const baseUrl =
    tag === 'latest'
      ? `https://github.com/${REPO}/releases/latest/download`
      : `https://github.com/${REPO}/releases/download/${tag}`;

  const tempDir = join(__dirname, '.tmp');
  const binDir = join(__dirname, '.bin');
  const archiveName = `ols-linux-${arch}.tar.gz`;
  const archivePath = join(tempDir, archiveName);
  const checksumsPath = join(tempDir, 'checksums.txt');

  if (!existsSync(tempDir)) mkdirSync(tempDir, { recursive: true });
  if (!existsSync(binDir)) mkdirSync(binDir, { recursive: true });

  await download(`${baseUrl}/${archiveName}`, archivePath);
  await download(`${baseUrl}/checksums.txt`, checksumsPath);

  const checksums = readFileSync(checksumsPath, 'utf8');
  const expected = checksumFor(archiveName, checksums);
  const actual = sha256(archivePath);
  if (expected !== actual) {
    throw new Error('binary checksum verification failed');
  }

  const tar = require('node:child_process').spawnSync('tar', ['-xzf', archivePath, '-C', tempDir], {
    stdio: 'inherit',
  });
  if (tar.status !== 0) {
    throw new Error('failed to extract binary archive');
  }

  const extracted = join(tempDir, 'ols');
  const destination = join(binDir, 'ols-linux');
  renameSync(extracted, destination);
  chmodSync(destination, 0o755);

  console.log('[ols npm] installed ols binary successfully');
}

run().catch((error) => {
  console.error(`[ols npm] postinstall failed: ${error.message}`);
  process.exit(1);
});
