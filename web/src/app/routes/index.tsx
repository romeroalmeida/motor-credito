import type { ReactNode } from 'react'
import { createBrowserRouter, Navigate, type RouteObject } from 'react-router-dom'
import { PainelLayout } from '@/app/layout/PainelLayout'
import { CadastroPage, EntrarPage, RotaProtegida } from '@/features/auth'
import type { Area } from '@/shared/utils/permissoes'
import { ExigirPapel } from './ExigirPapel'
import { Inicio } from './Inicio'

// Cada área vira um chunk próprio (lazy do React Router): o bundle inicial só leva entrar + layout.
function protegida(area: Area, carregar: () => Promise<ReactNode>): Pick<RouteObject, 'lazy'> {
  return { lazy: async () => ({ element: <ExigirPapel area={area}>{await carregar()}</ExigirPapel> }) }
}

export const rotas: RouteObject[] = [
  { path: '/entrar', element: <EntrarPage /> },
  { path: '/cadastro', element: <CadastroPage /> },
  {
    element: <RotaProtegida />,
    children: [
      {
        path: '/',
        element: <PainelLayout />,
        children: [
          { index: true, element: <Inicio /> },
          {
            path: 'visao-geral',
            ...protegida('visao-geral', async () => {
              const { VisaoGeralPage } = await import('@/features/visao-geral')
              return <VisaoGeralPage />
            }),
          },
          {
            path: 'consultas',
            ...protegida('consultas', async () => {
              const { ConsultasPage } = await import('@/features/consultas')
              return <ConsultasPage />
            }),
          },
          {
            path: 'consultas/:id',
            ...protegida('consultas', async () => {
              const { ConsultaDetalhePage } = await import('@/features/consultas')
              return <ConsultaDetalhePage />
            }),
          },
          {
            path: 'politica',
            ...protegida('politica', async () => {
              const { PoliticaPage } = await import('@/features/politica')
              return <PoliticaPage />
            }),
          },
          {
            path: 'chaves',
            ...protegida('chaves', async () => {
              const { ChavesPage } = await import('@/features/chaves')
              return <ChavesPage />
            }),
          },
          {
            path: 'equipe',
            ...protegida('equipe', async () => {
              const { EquipePage } = await import('@/features/equipe')
              return <EquipePage />
            }),
          },
          {
            path: 'auditoria',
            ...protegida('auditoria', async () => {
              const { AuditoriaPage } = await import('@/features/auditoria')
              return <AuditoriaPage />
            }),
          },
        ],
      },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
]

export const router = createBrowserRouter(rotas)
