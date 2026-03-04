import { build } from 'esbuild'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))

const entryPoints = [
  'src/diff/index.ts',
  'src/index/index.ts',
  'src/compare/index.ts',
  'src/review/index.ts',
]

const outdir = resolve(__dirname, '../internal/server/static/js')

await build({
  entryPoints,
  bundle: true,
  minify: true,
  sourcemap: false,
  format: 'iife',
  target: ['es2020'],
  outdir,
  // Name output files after parent directory (diff.js, index.js, etc.)
  entryNames: '[dir]',
})

console.log(`Built ${entryPoints.length} bundles to ${outdir}`)
