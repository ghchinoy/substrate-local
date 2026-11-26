// @ts-check
import { defineConfig, fontProviders } from 'astro/config';
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
      credits: false,
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
      customCss: ['./src/styles/docs.css'],
      components: {
        Head: './src/components/docs/Head.astro',
      },
      expressiveCode: {
        defaultProps: { frame: 'none' },
        styleOverrides: {
          borderRadius: '10px',
          borderColor: 'var(--md-sys-color-outline-variant)',
          codeBackground: 'var(--md-sys-color-surface-container-low)',
          frames: { shadowColor: 'transparent' },
        },
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
  fonts: [
    {
      // Google Sans Flex (OFL), self-hosted so the site makes zero external font calls.
      provider: fontProviders.local(),
      name: 'Google Sans Flex',
      cssVariable: '--font-display',
      fallbacks: ['system-ui', 'sans-serif'],
      options: {
        variants: [
          {
            weight: '100 1000',
            style: 'normal',
            display: 'swap',
            src: ['./src/assets/fonts/google-sans-flex.woff2'],
          },
        ],
      },
    },
    {
      provider: fontProviders.local(),
      name: 'Google Sans Code',
      cssVariable: '--font-mono',
      fallbacks: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      options: {
        variants: [
          {
            weight: '300 800',
            style: 'normal',
            display: 'swap',
            src: ['./src/assets/fonts/google-sans-code.woff2'],
          },
        ],
      },
    },
  ],
  vite: {
    build: {
      // Preserve light-dark() and @layer from @material/web gBreeze stylesheets.
      cssTarget: 'esnext',
    },
  },
});
