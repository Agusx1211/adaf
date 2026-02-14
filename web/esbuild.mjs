import * as esbuild from 'esbuild';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const outdir = path.resolve(__dirname, '..', 'internal', 'webserver', 'static');

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

if (isWatch) {
  const ctx = await esbuild.context(buildOptions);
  await ctx.watch();
  console.log('Watching for changes...');
} else {
  const result = await esbuild.build(buildOptions);
  if (result.errors.length) {
    console.error('Build failed:', result.errors);
    process.exit(1);
  }
  console.log('Build complete:', path.join(outdir, 'app.js'));
}
