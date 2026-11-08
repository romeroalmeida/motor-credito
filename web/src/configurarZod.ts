import { z } from 'zod'

// CSP sem 'unsafe-eval' (web/nginx.conf): o zod nem testa new Function (o teste vira violação de CSP no console).
z.config({ jitless: true })
