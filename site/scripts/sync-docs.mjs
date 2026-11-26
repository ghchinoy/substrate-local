// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Syncs canonical repository docs (docs/*.md, docs/diagrams/*.webp, CHANGELOG.md)
// into Starlight's content/public directories so the site stays single-source-of-truth.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const siteRoot = path.resolve(__dirname, '..');
const repoRoot = path.resolve(siteRoot, '..');
const base = '/substrate-local';
const ghBlob = 'https://github.com/ghchinoy/substrate-local/blob/main';

const pages = [
  {
    src: 'docs/quickstart-apple-container.md',
    dst: 'src/content/docs/guides/quickstart-apple-container.md',
    title: 'Apple container Quickstart (macOS)',
    description:
      'Run unmodified upstream AX on Apple Silicon macOS using native Apple container microVMs in ~30 seconds.',
  },
  {
    src: 'docs/quickstart-podman.md',
    dst: 'src/content/docs/guides/quickstart-podman.md',
    title: 'Rootless Podman Quickstart (macOS & Linux)',
    description:
      'Run unmodified upstream AX directly on rootless Podman with zero Docker and zero rootful VM required.',
  },
  {
    src: 'docs/choosing-a-runtime.md',
    dst: 'src/content/docs/guides/choosing-a-runtime.md',
    title: 'Choosing a Runtime: Local vs. Kind vs. GKE',
    description:
      'Compatibility boundary and decision guide for when to use substrate-local, Kind (ax local), or Google Cloud GKE.',
  },
  {
    src: 'docs/architecture.md',
    dst: 'src/content/docs/reference/architecture.md',
    title: 'Architecture & pkg/sublocal',
    description:
      'How substrate-local implements Agent Substrate v1.0 ControlServer and atenet-router over Apple container and Podman.',
  },
  {
    src: 'docs/go-app-integration.md',
    dst: 'src/content/docs/reference/go-app-integration.md',
    title: 'Integrating External Go Apps',
    description:
      'Drive unmodified upstream AX and substrate-local from external Go control planes with ephemeral audit jobs and stdin tar uploads.',
  },
  {
    src: 'docs/adr-001-no-local-egress-proxy.md',
    dst: 'src/content/docs/reference/adr-001-no-local-egress-proxy.md',
    title: 'ADR-001: Keep atenet-egress on Kind / GKE',
    description:
      'Architectural decision record explaining why atenet-egress and TLS-MITM credential injection stay strictly on Kind and GKE.',
  },
  {
    src: 'CHANGELOG.md',
    dst: 'src/content/docs/reference/changelog.md',
    title: 'Changelog',
    description: 'Release history for substrate-local.',
  },
];

const linkRewrites = new Map([
  ['quickstart-apple-container.md', `${base}/guides/quickstart-apple-container/`],
  ['docs/quickstart-apple-container.md', `${base}/guides/quickstart-apple-container/`],
  ['quickstart-podman.md', `${base}/guides/quickstart-podman/`],
  ['docs/quickstart-podman.md', `${base}/guides/quickstart-podman/`],
  ['choosing-a-runtime.md', `${base}/guides/choosing-a-runtime/`],
  ['docs/choosing-a-runtime.md', `${base}/guides/choosing-a-runtime/`],
  ['architecture.md', `${base}/reference/architecture/`],
  ['docs/architecture.md', `${base}/reference/architecture/`],
  ['go-app-integration.md', `${base}/reference/go-app-integration/`],
  ['docs/go-app-integration.md', `${base}/reference/go-app-integration/`],
  ['adr-001-no-local-egress-proxy.md', `${base}/reference/adr-001-no-local-egress-proxy/`],
  ['docs/adr-001-no-local-egress-proxy.md', `${base}/reference/adr-001-no-local-egress-proxy/`],
  ['../examples/task-audit-job.yaml', `${ghBlob}/examples/task-audit-job.yaml`],
  ['examples/task-audit-job.yaml', `${ghBlob}/examples/task-audit-job.yaml`],
]);

// 1. Copy dual-theme Lustrous WebP diagrams into site/public/diagrams/
const srcDiagramsDir = path.join(repoRoot, 'docs', 'diagrams');
const dstDiagramsDir = path.join(siteRoot, 'public', 'diagrams');
fs.mkdirSync(dstDiagramsDir, { recursive: true });
for (const entry of fs.readdirSync(srcDiagramsDir)) {
  if (entry.endsWith('.webp')) {
    fs.copyFileSync(path.join(srcDiagramsDir, entry), path.join(dstDiagramsDir, entry));
  }
}

// 2. Transform and sync Markdown pages with Starlight frontmatter and dual-theme diagram blocks
function transformMarkdown(raw, meta) {
  // Strip leading H1 since Starlight renders frontmatter title as H1.
  let body = raw.replace(/^#\s+[^\n]+\n+/, '');

  // Convert <picture> dual-theme diagram blocks into Starlight [data-theme]-aware blocks
  // so toggling Starlight's theme switcher updates diagrams immediately.
  body = body.replace(
    /<picture>\s*<source[^>]*srcset="(?:docs\/)?diagrams\/([^"]+)-dark\.webp">\s*<source[^>]*srcset="(?:docs\/)?diagrams\/([^"]+)-light\.webp">\s*<img alt="([^"]+)"[^>]*>\s*<\/picture>/g,
    (_, darkBase, lightBase, alt) => `<div class="lustrous-diagram">
  <img class="diagram-light" src="${base}/diagrams/${lightBase}-light.webp" alt="${alt}" />
  <img class="diagram-dark" src="${base}/diagrams/${darkBase}-dark.webp" alt="${alt}" />
</div>`
  );

  // Rewrite relative repo doc links to Starlight routes / GitHub URLs.
  body = body.replace(/\]\(([^)]+)\)/g, (match, target) => {
    const [urlPart, hashPart] = target.split('#');
    if (linkRewrites.has(urlPart)) {
      const mapped = linkRewrites.get(urlPart);
      return `](${mapped}${hashPart ? '#' + hashPart : ''})`;
    }
    return match;
  });

  const frontmatter = [
    '---',
    `title: ${JSON.stringify(meta.title)}`,
    `description: ${JSON.stringify(meta.description)}`,
    'editUrl: ' + JSON.stringify(`${ghBlob.replace('/blob/', '/edit/')}/${meta.src}`),
    '---',
    '',
  ].join('\n');

  return frontmatter + body;
}

for (const page of pages) {
  const srcPath = path.join(repoRoot, page.src);
  const dstPath = path.join(siteRoot, page.dst);
  fs.mkdirSync(path.dirname(dstPath), { recursive: true });
  const raw = fs.readFileSync(srcPath, 'utf8');
  fs.writeFileSync(dstPath, transformMarkdown(raw, page), 'utf8');
}

console.log(`Synced ${pages.length} docs and diagrams into site/`);
