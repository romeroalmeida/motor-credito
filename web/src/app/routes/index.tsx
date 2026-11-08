import { createBrowserRouter, Navigate, type RouteObject } from 'react-router-dom'
import { PainelLayout } from '@/app/layout/PainelLayout'
import { EntrarPage, RotaProtegida } from '@/features/auth'
import { ChavesPage } from '@/features/chaves'
import { ConsultaDetalhePage, ConsultasPage } from '@/features/consultas'
import { EquipePage } from '@/features/equipe'
import { PoliticaPage } from '@/features/politica'
import { VisaoGeralPage } from '@/features/visao-geral'
import { ExigirPapel } from './ExigirPapel'

export const rotas: RouteObject[] = [
  { path: '/entrar', element: <EntrarPage /> },
  {
    element: <RotaProtegida />,
    children: [
      {
        path: '/',
        element: <PainelLayout />,
        children: [
          { index: true, element: <Navigate to="/visao-geral" replace /> },
          { path: 'visao-geral', element: <VisaoGeralPage /> },
          {
            path: 'consultas',
            element: (
              <ExigirPapel area="consultas">
                <ConsultasPage />
              </ExigirPapel>
            ),
          },
          {
            path: 'consultas/:id',
            element: (
              <ExigirPapel area="consultas">
                <ConsultaDetalhePage />
              </ExigirPapel>
            ),
          },
          {
            path: 'politica',
            element: (
              <ExigirPapel area="politica">
                <PoliticaPage />
              </ExigirPapel>
            ),
          },
          {
            path: 'chaves',
            element: (
              <ExigirPapel area="chaves">
                <ChavesPage />
              </ExigirPapel>
            ),
          },
          {
            path: 'equipe',
            element: (
              <ExigirPapel area="equipe">
                <EquipePage />
              </ExigirPapel>
            ),
          },
        ],
      },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
]

export const router = createBrowserRouter(rotas)
