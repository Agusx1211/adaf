import * as esbuild from 'esbuild';
import fs from 'node:fs/promises';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const outdir = path.resolve(__dirname, '..', 'internal', 'webserver', 'static');
const htmlTemplatePath = path.resolve(__dirname, 'src', 'index.html');
const htmlOutputPath = path.join(outdir, 'index.html');

const isProd = process.argv.includes('--prod');
const isWatch = process.argv.includes('--watch');

const buildOptions = {
  entryPoints: [path.resolve(__dirname, 'src', 'index.jsx')],
  bundle: true,
  outfile: path.join(outdir, 'app.js'),
  format: 'iife',
  platform: 'browser',
  target: ['es2020'],
  jsx: 'automatic',
  jsxImportSource: 'react',
  minify: isProd,
  sourcemap: !isProd,
  define: {
    'process.env.NODE_ENV': isProd ? '"production"' : '"development"',
  },
  loader: {
    '.jsx': 'jsx',
    '.js': 'jsx',
    '.css': 'css',
  },
  external: [],
};

async function copyHtmlTemplate() {
  await fs.copyFile(htmlTemplatePath, htmlOutputPath);
}

if (isWatch) {
  await copyHtmlTemplate();
  const ctx = await esbuild.context(buildOptions);
  await ctx.watch();
  console.log('Watching for changes...');
} else {
  await copyHtmlTemplate();
  const result = await esbuild.build(buildOptions);
  if (result.errors.length) {
    console.error('Build failed:', result.errors);
    process.exit(1);
  }
  console.log('Build complete:', path.join(outdir, 'app.js'), 'and', path.join(outdir, 'app.css'), 'and', htmlOutputPath);
}
