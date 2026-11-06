import { writeFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    svelte(),
    {
      // Vite vide dist/ à chaque build : on y remet .gitkeep, sans quoi `//go:embed all:frontend/dist`
      // échoue ("no matching files") sur un clone neuf, avant la première compilation du front.
      name: 'keep-dist-gitkeep',
      closeBundle() {
        try { writeFileSync('dist/.gitkeep', '') } catch { /* dist absent : rien à faire */ }
      },
    },
  ],
})
