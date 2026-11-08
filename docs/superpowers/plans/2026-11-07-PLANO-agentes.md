# Plano de execução por agentes — motor-credito

Desenho: `docs/superpowers/specs/2026-11-07-motor-credito-design.md` (**leia antes de tudo**).
Este arquivo é o **contrato**: pastas, donos, interfaces e regras. O que não está aqui segue o desenho.

## Regras para todo agente

- Português do Brasil em nomes de domínio, mensagens e comentários. Código Go idiomático (gofmt, erros com `%w`).
- **Só toque nas pastas que são suas** (tabela abaixo). Precisou mexer em outra? Pare e reporte no relatório.
- Sem dependência nova fora da lista da fundação. Precisou? Reporte; não instale.
- Segurança fail-closed: na dúvida, nega. Variável de ambiente esquecida produz o estado seguro, nunca o inseguro.
- Dinheiro sempre `int64` em centavos; taxa em pontos-base (`int`, 290 = 2,90% a.m.). Nunca `float` em valor monetário guardado ou devolvido.
- CPF/CNPJ nunca em log nem em texto puro no banco (ver desenho §4).
- Testes: em tabela (Go), nos limites exatos. Cada teste de regra precisa pegar a implementação errada: faça **mutação manual** (quebre a regra, confirme o vermelho, desfaça) nas regras de dinheiro, motor, antifraude, HMAC e isolamento. Liste no relatório as mutações feitas.
- Antes de terminar: `go vet ./...`, `test -z "$(gofmt -l .)"`, `go test ./...` (ou `npm run lint && npm test && npm run build` no web) verdes.
- Commit na sua branch com mensagem em pt-BR e a linha `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Adicione arquivos por caminho (nunca `git add -A`). Nunca `push`.
- Relatório final em até ~15 linhas: o que foi feito (com hash), o que ficou faltando, riscos, mutações provadas.

## Stack fixada pela fundação

- Go: módulo `github.com/romeroalmeida/motor-credito`, diretiva `go 1.25` (`go mod edit -go=1.25`).
- Dependências Go: `github.com/go-chi/chi/v5`, `github.com/jackc/pgx/v5`, `github.com/google/cel-go`, `golang.org/x/crypto` (argon2), `github.com/pquerna/otp`, `github.com/oklog/ulid/v2`, `github.com/testcontainers/testcontainers-go` + módulo postgres.
- Postgres 17 via `docker compose`. Migrations em SQL puro em `migrations/` aplicadas por um runner simples próprio (`internal/plataforma/migrar`), sem ferramenta externa.
- Web (`web/`): npm, React 19 + TS + Vite, React Router Dom, Zustand, React Query + Axios, Tailwind + shadcn/ui, React Hook Form + Zod, Vitest + Testing Library, Playwright (e2e), MSW (mocks), ESLint + Prettier, aliases `@/*`. Estrutura **feature-based** (skill `front-architecture`).

## Donos de pasta

| Pasta | Dono |
|---|---|
| `go.mod`, `go.sum`, `docker-compose.yml`, `Makefile`, `.github/`, `migrations/`, `internal/plataforma/`, `internal/documento/`, `internal/dinheiro/`, `api/openapi.yaml`, `web/package.json` + lockfile + configs | **F — Fundação** |
| `internal/tenant/`, `internal/painelauth/` | **A — Tenant e autenticação** |
| `internal/politica/` | **B — Política e motor** |
| `internal/bureau/`, `internal/antifraude/` | **C — Bureau e antifraude** |
| `web/src/` (exceto o que F criar como base) | **D — Painel** |
| `cmd/api/`, `internal/consulta/`, `internal/auditoria/`, `internal/httpapi/`, `internal/repositorio/` | **E — Integração** (depois da onda 1) |

## Ondas

1. **F sozinho** na `main`. Ver "Tarefa F".
2. **A, B, C, D em paralelo**, cada um num worktree e branch próprios (`onda1-a-tenant`, `onda1-b-politica`, `onda1-c-antifraude`, `onda1-d-painel`). Nenhum arquivo em comum.
3. **E** faz merge das quatro branches na `main` e liga tudo: HTTP, Postgres, orquestração, auditoria e troca do MSW pela API real.
4. **Conferência** por agentes novos, sem histórico. Resposta no formato Feito / Falta com `arquivo:linha`. Correções por outro agente.

## Interfaces (contrato entre módulos)

Os tipos abaixo são **exatamente** os que cada dono expõe. E depende deles.

### F — `internal/documento`
```go
type Tipo string // "cpf" | "cnpj"
// Normalizar remove máscara, põe letras em maiúscula e valida dígitos verificadores.
// CNPJ alfanumérico (Receita 2026): 12 primeiras [0-9A-Z], 2 últimas numéricas, valor do caractere = ASCII-48,
// módulo 11 de sempre. Recusa sequência repetida. Exemplo oficial 12ABC34501DE35 é válido.
func Normalizar(entrada string) (doc string, tipo Tipo, err error)
func Mascarar(doc string) string // "***.456.789-**" / "**.ABC.345/****-**"
```

### F — `internal/dinheiro`
```go
type Centavos = int64
// Parcela pela tabela Price; taxaBps em pontos-base ao mês (290 = 2,90%). taxa 0 → valor/prazo.
// Cálculo em big.Rat, arredondamento meio-para-cima no centavo. Testar contra valores de referência.
func Parcela(valor Centavos, prazoMeses int, taxaBps int) Centavos
```

### F — `internal/plataforma`
Config por ambiente (fail-closed: falta `CHAVE_HMAC_DOC`, `CHAVE_CIFRA_DOC`, `DATABASE_URL` → o binário não sobe), pool pgx, runner de migrations, `ulid` com prefixo (`NovoID("cons")`), helpers de cifra: `HashDoc(chave, doc) []byte` (HMAC-SHA256) e `Cifrar/Decifrar` (AES-256-GCM).

### A — `internal/tenant` e `internal/painelauth`
```go
// tenant
type Tenant struct{ ID, Nome string; LimitePorMinuto int; IPsPermitidos []netip.Prefix }
type ChaveCriada struct{ ID, Segredo string } // segredo completo "ck_live_<id>_<segredo>" aparece só aqui
type Servico struct{ /* recebe *pgxpool.Pool */ }
func (s *Servico) CriarChave(ctx, tenantID string) (ChaveCriada, error)
func (s *Servico) RevogarChave(ctx, tenantID, chaveID string) error
func (s *Servico) ListarChaves(ctx, tenantID string) ([]Chave, error)
// Middleware da API pública: Bearer + X-Timestamp (±5 min) + X-Assinatura (hex HMAC-SHA256 de timestamp+"."+corpo,
// chave = segredo da chave de API) + limite por minuto + IP permitido. Comparação em tempo constante.
// Em sucesso põe o tenant no contexto. Em falha: 401 com {"erro":"nao_autorizado"} (sem dizer qual parte falhou), 429 no limite.
func (s *Servico) MiddlewareAPI(next http.Handler) http.Handler
func TenantDoContexto(ctx context.Context) (Tenant, bool)
// Idempotência: mesma chave+mesmo hash de corpo devolve resposta guardada; corpo diferente → ErrIdempotenciaConflito (409).
func (s *Servico) Idempotencia(ctx, tenantID, chave string, hashCorpo []byte) (respostaGuardada []byte, achou bool, err error)
func (s *Servico) GuardarIdempotencia(ctx, tenantID, chave string, hashCorpo, resposta []byte) error

// painelauth
type Papel string // "admin" | "analista" | "auditor"
type Sessao struct{ UsuarioID, TenantID string; Papel Papel }
// Rotas: POST /painel/auth/entrar (email+senha → exige TOTP), POST /painel/auth/2fa, POST /painel/auth/sair,
// GET /painel/auth/eu, GET/POST/DELETE /painel/chaves, GET/POST /painel/equipe (convite gera link de cadastro com TOTP).
func (h *Handlers) Rotas(r chi.Router)
// Middleware de sessão: cookie httpOnly+Secure+SameSite=Strict; CSRF por cabeçalho X-CSRF (double submit) em método que escreve.
// Tenant vem SEMPRE da sessão. Sem sessão → 401.
func (h *Handlers) MiddlewareSessao(next http.Handler) http.Handler
func ExigirPapel(p ...Papel) func(http.Handler) http.Handler // papel errado → 403; recurso de outro tenant → 404 (responsabilidade do repositório)
func SessaoDoContexto(ctx context.Context) (Sessao, bool)
```
Senha argon2id. TOTP obrigatório. Login com rate limit por e-mail+IP. Cookie `Secure` liga por padrão; só desliga com `COOKIE_INSEGURO_DEV=on` **e** ambiente diferente de produção (duas travas).

### B — `internal/politica`
```go
// Modelo editável (o que o painel salva como JSON).
type Politica struct {
    Cortes          []Corte          `json:"cortes"`
    Faixas          []Faixa          `json:"faixas"`
    Contraproposta  Contraproposta   `json:"contraproposta"`
    ExpressaoAvancada bool           `json:"expressao_avancada"` // true = condições vieram escritas em CEL pelo cliente
}
type Condicao struct { Campo, Operador string; Valor any }      // editor visual: {"score","<=",100}
type Corte struct { Codigo, Texto string; Condicoes []Condicao; CEL string } // E entre condições; ou CEL direto
type Faixa struct {
    Rotulo        string     `json:"rotulo"` // "A","B","C"...
    Condicoes     []Condicao `json:"condicoes"`; CEL string `json:"cel"`
    Nega          bool       `json:"nega"`
    TaxaBps       int        `json:"taxa_bps"`
    LimiteFixo    int64      `json:"limite_fixo"`      // centavos; usado se não houver renda ou multiplicador 0
    MultiploRenda int        `json:"multiplo_renda_x10"` // 30 = 3,0× renda
}
type Contraproposta struct { Ativa bool; PrazoMaximo int; AcrescimoTaxaBps int }

type Contexto struct {
    Score int; Restricoes, Protestos int; DividaTotal int64
    Renda *int64; Idade *int
    PedidoValor int64; PedidoPrazo int
    FraudeRisco int; FraudeSinais []string
}
type Status string // aprovado | negado | contraproposta
type Motivo struct{ Codigo, Texto string }
type Oferta struct{ Valor int64; Prazo, TaxaBps int; Parcela int64 }
type RegraAvaliada struct{ Bloco, Rotulo, Expressao string; Bateu bool }
type Decisao struct {
    Status Status; FaixaRisco string; Oferta *Oferta; Motivos []Motivo
    Trilha []RegraAvaliada // usado na tela de detalhe do painel
}
type Compilada struct{ /* programas CEL prontos */ }
// Compilar valida tudo: campos conhecidos, operadores permitidos (== != < <= > >= in), tipos via CEL checker,
// has() obrigatório para Renda/Idade, limite de custo e de tamanho, ao menos uma faixa. Erro lista a regra e o motivo.
func Compilar(p Politica) (*Compilada, error)
func (c *Compilada) Avaliar(ctx Contexto) (Decisao, error)
func ModeloEmprestimoPF() Politica
```
Ordem: cortes (primeiro que bate nega) → faixas (primeira que bate; `Nega` nega) → limite = `min(LimiteFixo, renda*mult/10)` (sem renda ou mult 0 → LimiteFixo) → se pedido ≤ limite: aprovado; senão contraproposta (se ativa: oferta = limite, prazo = min(pedido.prazo, PrazoMaximo>0?PrazoMaximo:pedido.prazo), taxa+acréscimo) ou negado com `LIMITE_FAIXA`. Nenhuma faixa bate → negado `SEM_FAIXA`. Parcela via `dinheiro.Parcela`. Avaliação com custo limitado (`cel.CostLimit`) e sem chamadas a funções fora da lista.

### C — `internal/bureau` e `internal/antifraude`
```go
// bureau
type Situacao string // regular | irregular | falecido | inativo
type Resultado struct {
    Score int; Restricoes, Protestos int; DividaTotal int64
    Nome string; Nascimento string /* AAAA-MM-DD */; Situacao Situacao; Renda *int64
}
type Bureau interface{ Consultar(ctx context.Context, doc string) (Resultado, error) }
var ErrIndisponivel = errors.New("bureau indisponível")
// Simulado: determinístico pelo documento (mesmo doc → mesmo resultado), perfis variados incluindo campos ausentes.
// Documentos-fixture documentados em README do pacote: um falecido, um irregular, um score baixo, um score alto,
// um com nome divergente para teste, um que simula indisponibilidade/lentidão (respeita ctx).
// Fixtures com dígitos válidos mas óbvios de ficção (CNPJ alfanumérico com "EXEMPLO..." ; CPFs de teste gerados).
func NovoSimulado(atraso time.Duration) Bureau

// antifraude
type Entrada struct {
    DocHash []byte; Tipo string // "cpf"|"cnpj"
    Nome, Nascimento, Email, IP, DeviceID string // "" = não enviado
    Bureau bureau.Resultado
}
type Resultado struct{ Risco int; Sinais, Executadas, Puladas []string }
// Contador é implementado por E sobre o Postgres; C testa com um fake em memória.
type Contador interface {
    ConsultasDoc24h(ctx context.Context, docHash []byte) (int, error)      // em todos os tenants
    DocsPorDispositivo24h(ctx context.Context, deviceID string) (int, error)
}
func Avaliar(ctx context.Context, e Entrada, c Contador) (Resultado, error)
```
Checagens e nomes de sinal: tabela do desenho §6. Pesos em constantes nomeadas. Risco = soma limitada a 100. Comparação de nome normalizada (sem acento, maiúscula, ignorando preposições; divergente se nenhum token de sobrenome bate). IP: listas CIDR embutidas pequenas de datacenter e faixa brasileira (`shortcut:` com comentário). E-mail descartável: lista embutida. Situação `irregular`/`inativo` → sinal; `falecido` é tratado por E como `documento_invalido` (C apenas não pontua).

### E — integração (onda 3)
- `internal/repositorio`: Postgres com tenant no construtor (`ParaTenant(pool, tenantID)`) + RLS (`SET LOCAL app.tenant_id`). Implementa `antifraude.Contador`.
- `internal/auditoria`: grava a decisão com hash encadeado, na mesma transação da consulta.
- `internal/consulta`: orquestra (desenho §3), bureau + antifraude em paralelo com `context.WithTimeout(2s)`.
- `internal/httpapi`: router chi; `/v1/consultas`; `/painel/consultas`, `/painel/consultas/{id}`, `/painel/politicas` (rascunho, testar, publicar, versões, diferença, reverter), `/painel/visao-geral`; monta `painelauth.Rotas`; cabeçalhos de segurança; corpo limitado a 64 KB; `/saude`.
- `cmd/api`: sobe tudo; `cmd/semente`: cria tenant demo + admin demo **só** com `SEED_DEMO=on` e ambiente ≠ produção, imprime o segredo TOTP e uma chave de API.
- Troca o MSW pela API real no web (MSW continua só nos testes).

## Tarefa F — Fundação (sozinho, na `main`)

1. `go mod init`, diretiva 1.25, todas as dependências Go da lista (adicione com `go get` e um arquivo `internal/plataforma/deps.go` com imports em branco `_` apenas para `go mod tidy` não as remover; E apaga depois).
2. `internal/documento`, `internal/dinheiro`, `internal/plataforma` com testes (mutação provada no CNPJ alfanumérico — ASCII-48 vs base 36 —, na Price e no arredondamento).
3. **Todo o schema** em `migrations/0001_inicial.sql` (desenho §4–§8): tenants, chaves_api (hash), usuarios, associacoes (usuario, tenant, papel), sessoes, totp, convites, idempotencia, politicas, politica_versoes (imutável), consultas (doc_hash, doc_cifrado, entrada sem PII em claro, decisao jsonb, politica_versao), auditoria (hash_anterior, hash), contadores de antifraude por índice. **RLS** habilitada em toda tabela com `tenant_id` usando `current_setting('app.tenant_id', true)`; papel de banco da aplicação `motor_app` sem `UPDATE/DELETE` em `auditoria` e `politica_versoes`. Teste de integração (testcontainers) que prova: RLS nega leitura cruzada; `motor_app` não consegue `UPDATE` na auditoria.
4. `docker-compose.yml` (postgres 17 + api + web), `Makefile` (`make dev`, `make test`, `make lint`), `.env.example` sem valores reais, CI em `.github/workflows/ci.yml` (go vet, gofmt, go test, govulncheck, gosec, web lint/test/build, npm audit --audit-level=high, gitleaks).
5. `api/openapi.yaml` completo para `/v1/consultas` e todas as rotas `/painel/*` listadas acima (requisição, resposta, erros) — é o contrato do painel D.
6. `web/`: Vite + React + TS com **todas** as dependências instaladas e configuradas (lista da stack), estrutura feature-based criada (`app/`, `features/{auth,visao-geral,consultas,politica,chaves,equipe}` com barrel, `shared/`, `api/` com apiClient Axios `withCredentials` + CSRF, `state/` com store de sessão), MSW configurado com handlers vazios por feature, tokens de tema do desenho §8 em `shared/styles/` (fundo cinza claro, cards brancos raio ~20px, sombra difusa, quase-preto de destaque, verde/vermelho/roxo/âmbar como sinal), shadcn inicializado, layout com menu lateral com **todos** os links e páginas placeholder, e componente compartilhado `StatusDecisao` (bolinha + texto) usado por várias features.
7. Commit na `main`. Relatório.

## Tarefas da onda 1

- **A:** tudo da seção A + testes (HMAC: assinatura errada, timestamp vencido/futuro, chave revogada, corpo alterado; idempotência 409; sessão sem CSRF recusada; papel errado 403; TOTP errado; login com rate limit; cookie inseguro só com as duas travas). Testes de integração com testcontainers usando o schema da fundação.
- **B:** tudo da seção B + testes em tabela (limites 300/301 e 600/601 do modelo PF; contraproposta ligada/desligada; renda ausente → limite fixo; regra com `renda` sem `has()` recusada; expressão com custo excessivo recusada; operador fora da lista recusado; faixa `Nega`; nenhum corte bate). Mutação provada em ordem de cortes, ordem de faixas, `min` do limite e `<=` do pedido.
- **C:** tudo da seção C + testes (cada checagem pulada quando o dado falta; velocidade no limite N e N+1; nome com acento/caixa/preposição; risco limitado a 100; simulado determinístico e respeitando ctx/atraso).
- **D:** painel completo contra o `api/openapi.yaml` com MSW: Entrar (senha → 2FA), Visão geral, Consultas (lista com filtros + detalhe com trilha de regras e versão), Política (abas Corte/Faixas/Contraproposta; editor visual de condições; modo avançado CEL; rascunho → testar com CPF → publicar com confirmação; histórico com diferença e reverter), Chaves (segredo mostrado uma vez, girar, revogar, IPs), Equipe. Direção visual do desenho §8 (ver `docs/referencias/ui-pinterest-695032155015577928.jpg`), responsivo, acessível, cor nunca sozinha. Use a skill `front-architecture` (já respondida: os defaults acima valem, não pergunte) e `cleancode`. Vitest nos componentes do editor de política e na máscara de CPF/CNPJ. Não crie e2e (fica para E, com API real).

## Notas para E (vindas dos relatórios da onda 1)

**Ordem bureau → antifraude (corrige o contrato).** `antifraude.Entrada` exige o `bureau.Resultado`, então não há paralelismo: o bureau roda primeiro e o antifraude depois, os dois dentro do mesmo `context.WithTimeout(2s)`. Se o bureau falhar ou estourar o tempo, a resposta é `negado` com `BUREAU_INDISPONIVEL` e o antifraude nem roda. Se o `Contador` der erro, `Avaliar` aborta: a resposta também é negada, nunca aprovada.

**Política (B, `bbbceff`)**
- `Avaliar` pode devolver erro (custo estourado, pedido inválido) → `negado`, nunca aprovado.
- `FaixaRisco` vazia vira `null` na resposta.
- `documento_invalido` (dígito inválido ou bureau dizendo `falecido`) e `BUREAU_INDISPONIVEL` ficam com E.
- `ErroPolitica{Problemas}` preenche os `detalhes` do 422 na publicação.
- O módulo do CEL é `cel.dev/cel-go`, como está no go.mod.

**Antifraude e bureau (C, `80966c9`)**
- `bureau.Consultar` devolve `documento.ErrInvalido` quando o documento é inválido.
- Os documentos de fixture estão em `internal/bureau/README.md`.

**Tenant e auth (A, `a3359c7`)**
- Montagem: `h := painelauth.Novo(poolApp, tenant.NovoServico(poolApp), painelauth.Opcoes{ChaveCifra: cfg.ChaveCifraDoc, CookieInseguro: painelauth.CookieInseguroPermitido(os.Getenv("COOKIE_INSEGURO_DEV"), cfg.Ambiente), URLPainel: ...})`, depois `h.Rotas(r)` (registra os caminhos completos `/painel/...`). As outras rotas do painel usam `h.MiddlewareSessao` e `painelauth.ExigirPapel`.
- `servico.MiddlewareAPI` já lê o corpo (limite de 64 KB) e devolve o corpo ao handler. Ele põe `TenantDoContexto` e `ChaveDoContexto` no contexto (o id da chave serve para o ator `api:<id>` na auditoria).
- A chave do HMAC é a string inteira `ck_live_<id>_<segredo>`; o helper é `tenant.Assinar`.
- Idempotência: `ErrIdempotenciaConflito` → 409, `ErrIdempotenciaChaveInvalida` → 400. E calcula o hash do corpo.
- `tenant.EmTenant(ctx, pool, tenantID, fn)` faz o `SET LOCAL app.tenant_id`; o repositório deve reusar.
- O IP vem de `r.RemoteAddr`. Não confie em `X-Forwarded-For` sem uma lista de proxies confiáveis.
- O convite para um e-mail que já tem conta responde 409: acrescente essa resposta no OpenAPI.
