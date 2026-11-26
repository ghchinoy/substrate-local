// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLlmsTxt from 'starlight-llms-txt';

export default defineConfig({
  site: 'https://ghchinoy.github.io',
  base: '/substrate-local',
  output: 'static',
  integrations: [
    starlight({
      title: 'substrate-local',
      description:
        'Zero-Kubernetes local Agent Substrate v1.0 Control API + atenet-router daemon backed by macOS Apple container and rootless Podman.',
      favicon: '/favicon.svg',
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/ghchinoy/substrate-local',
        },
      ],
      editLink: {
        baseUrl: 'https://github.com/ghchinoy/substrate-local/edit/main/site/',
      },
      customCss: ['./src/styles/custom.css'],
      expressiveCode: {
        defaultProps: { frame: 'none' },
      },
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'macOS Apple container', slug: 'guides/quickstart-apple-container' },
            { label: 'Rootless Podman (macOS & Linux)', slug: 'guides/quickstart-podman' },
            { label: 'Choosing a Runtime (Local vs. Kind vs. GKE)', slug: 'guides/choosing-a-runtime' },
          ],
        },
        {
          label: 'Architecture & Integration',
          items: [
            { label: 'Architecture & pkg/sublocal', slug: 'reference/architecture' },
            { label: 'Integrating External Go Apps', slug: 'reference/go-app-integration' },
            { label: 'ADR-001: No Local Egress Proxy', slug: 'reference/adr-001-no-local-egress-proxy' },
          ],
        },
        {
          label: 'Project',
          items: [
            { label: 'Changelog', slug: 'reference/changelog' },
            {
              label: 'AX & Substrate Concepts (ax-experiments)',
              link: 'https://github.com/ghchinoy/ax-experiments/tree/main/concepts',
              attrs: { target: '_blank', rel: 'noopener noreferrer' },
            },
            {
              label: 'GitHub Releases',
              link: 'https://github.com/ghchinoy/substrate-local/releases',
              attrs: { target: '_blank', rel: 'noopener noreferrer' },
            },
          ],
        },
      ],
      plugins: [
        starlightLlmsTxt({
          projectName: 'substrate-local',
          description:
            'Zero-Kubernetes local Agent Substrate v1.0 Control API + atenet-router daemon backed by macOS Apple container and rootless Podman for unmodified upstream google/ax.',
        }),
      ],
    }),
  ],
});
