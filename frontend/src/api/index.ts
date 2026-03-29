import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
  timeout: 120000, // 120s for long-running operations like AI analysis
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('cqa_access_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Shared refresh promise to prevent multiple concurrent refresh calls
let refreshPromise: Promise<string> | null = null

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config
    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true
      try {
        // If a refresh is already in-flight, wait for it instead of firing another
        if (!refreshPromise) {
          refreshPromise = axios
            .post('/api/v1/auth/refresh', {}, { withCredentials: true })
            .then(({ data }) => {
              localStorage.setItem('cqa_access_token', data.access_token)
              return data.access_token
            })
            .finally(() => {
              refreshPromise = null
            })
        }
        const newToken = await refreshPromise
        originalRequest.headers.Authorization = `Bearer ${newToken}`
        return api(originalRequest)
      } catch {
        localStorage.removeItem('cqa_access_token')
        localStorage.removeItem('cqa_refresh_token')
        window.location.href = '/login'
        return Promise.reject(error)
      }
    }
    return Promise.reject(error)
  },
)

export default api
