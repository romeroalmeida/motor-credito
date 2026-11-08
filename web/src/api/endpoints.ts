// Rotas do painel em api/openapi.yaml.
export const endpoints = {
  auth: {
    entrar: '/painel/auth/entrar',
    doisFatores: '/painel/auth/2fa',
    cadastro: '/painel/auth/cadastro',
    sair: '/painel/auth/sair',
    eu: '/painel/auth/eu',
  },
  visaoGeral: '/painel/visao-geral',
  auditoria: {
    lista: '/painel/auditoria',
    integridade: '/painel/auditoria/integridade',
  },
  consultas: {
    busca: '/painel/consultas/busca',
    detalhe: (id: string) => `/painel/consultas/${encodeURIComponent(id)}`,
  },
  politicas: {
    atual: '/painel/politicas',
    rascunho: '/painel/politicas/rascunho',
    testar: '/painel/politicas/testar',
    publicar: '/painel/politicas/publicar',
    versoes: '/painel/politicas/versoes',
    versao: (versao: number) => `/painel/politicas/versoes/${versao}`,
    reverter: (versao: number) => `/painel/politicas/versoes/${versao}/reverter`,
    diferenca: '/painel/politicas/diferenca',
  },
  chaves: {
    lista: '/painel/chaves',
    chave: (id: string) => `/painel/chaves/${encodeURIComponent(id)}`,
    girar: (id: string) => `/painel/chaves/${encodeURIComponent(id)}/girar`,
    ips: '/painel/chaves/ips',
  },
  equipe: '/painel/equipe',
  simulador: '/painel/simulador/consultas',
} as const
