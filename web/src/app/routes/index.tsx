import { createBrowserRouter, Navigate, type RouteObject } from 'react-router-dom'
import { PainelLayout } from '@/app/layout/PainelLayout'
import { EntrarPage } from '@/features/auth'
import { ChavesPage } from '@/features/chaves'
import { ConsultasPage } from '@/features/consultas'
import { EquipePage } from '@/features/equipe'
import { PoliticaPage } from '@/features/politica'
import { VisaoGeralPage } from '@/features/visao-geral'

export const rotas: RouteObject[] = [
  { path: '/entrar', element: <EntrarPage /> },
  {
    path: '/',
    element: <PainelLayout />,
    children: [
      { index: true, element: <Navigate to="/visao-geral" replace /> },
      { path: 'visao-geral', element: <VisaoGeralPage /> },
      { path: 'consultas', element: <ConsultasPage /> },
      { path: 'politica', element: <PoliticaPage /> },
      { path: 'chaves', element: <ChavesPage /> },
      { path: 'equipe', element: <EquipePage /> },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
]

export const router = createBrowserRouter(rotas)
