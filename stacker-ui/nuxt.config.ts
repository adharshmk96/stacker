// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  devtools: { enabled: true },
  modules: ['@nuxt/ui'],

  // The build is embedded into the stacker binary and served as static files,
  // so there is no Node server at runtime to render on.
  ssr: false,

  runtimeConfig: {
    public: {
      // Same origin: in production the Go server serves both the UI and /api.
      // In dev the nitro proxy below stands in for that.
      apiBase: '/api'
    }
  },

  nitro: {
    devProxy: {
      '/api': {
        target: 'http://localhost:8080/api',
        changeOrigin: true
      }
    }
  },

  css: ['~/assets/css/main.css'],
  colorMode: {
    preference: 'dark'
  },
  ui: {
    theme: {
      // `stacker` is the cyan ramp defined in assets/css/main.css
      colors: ['primary', 'secondary', 'success', 'info', 'warning', 'error', 'stacker']
    }
  }
})
