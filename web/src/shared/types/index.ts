// Tipos do contrato api/openapi.yaml. Não edite api.gen.ts: rode `npm run gerar:tipos`.
import type { components, paths } from './api.gen'

export type Schemas = components['schemas']
export type ApiPaths = paths
export type StatusDecisao = Schemas['StatusDecisao']
export type Papel = Schemas['Papel']
