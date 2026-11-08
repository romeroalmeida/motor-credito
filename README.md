# Motor de Crédito

Uma empresa envia um CPF ou CNPJ e recebe, numa chamada síncrona, uma **decisão de crédito precificada**: score,
faixa de risco, decisão (aprovado, negado, contraproposta, pré-aprovado ou documento inválido), oferta com valor,
prazo, taxa e parcela, o limite disponível, os motivos e o resultado do antifraude. Sem valor pedido, responde quanto
está disponível para aquele cliente (pré-aprovação). Cada cliente escreve a própria política de crédito num painel,
testa contra um documento de exemplo e publica uma versão nova sem deploy.

Projeto de portfólio, desenhado para virar produto: as fontes de dados ficam atrás de uma interface, então trocar o
bureau simulado por um real não mexe no motor.

## Arquitetura

```
                 cliente (sistema da empresa)                         analista / admin
                            │                                                │
      POST /v1/consultas    │  Bearer ck_live_<id> + HMAC                    │  cookie de sessão + CSRF
                            ▼                                                ▼
                 ┌──────────────────────────────────────────────────────────────────┐
                 │  nginx  (painel React estático + proxy de /v1 e /painel)        │
                 └──────────────────────────────────┬───────────────────────────────┘
                                                    ▼
 ┌────────────────────────────────── cmd/api (um binário Go) ───────────────────────────────────┐
 │  httpapi ── tenant (chave, HMAC, idempotência)      painelauth (argon2id, TOTP, papéis)      │
 │     │                                                                                        │
 │     ▼                                                                                        │
 │  consulta ──► documento (dígito) ──► bureau ──► antifraude ──► politica (CEL) ──► dinheiro   │
 │     │                                 │  simulado + Serpro (CPF) + BrasilAPI    (Price)      │
 │     ▼                                                                                        │
 │  repositorio + auditoria (mesma transação, hash encadeado)                                   │
 └────────────────────────────────────────────┬─────────────────────────────────────────────────┘
                                              ▼
                       PostgreSQL 17 (RLS por tenant, auditoria só de inserção)

 cmd/retencao (agendado): anonimiza consultas mais velhas que RETENCAO_DIAS
```

Fluxo de uma consulta: valida o documento → bureau (máx. 2 s) → antifraude → carrega a versão publicada da política
→ regras de corte → faixas de preço → contraproposta → grava decisão e auditoria na mesma transação → responde.

Detalhes: [`docs/superpowers/specs/2026-11-07-motor-credito-design.md`](docs/superpowers/specs/2026-11-07-motor-credito-design.md)
e o contrato em [`api/openapi.yaml`](api/openapi.yaml).

## Decisões e porquês

| Decisão | Porquê |
|---|---|
| **Go** no backend | Binário estático pequeno, poucas dependências, tipagem forte e concorrência nativa para falar com fontes externas. |
| **Monólito modular** | Um desenvolvedor. Microserviços trariam rede autenticada entre serviços e consistência distribuída sem ganho agora. Os módulos só se falam por interface Go e cada um tem as próprias tabelas, então extrair um serviço é trocar a implementação da interface. |
| **Política em CEL** (`cel-go`) | O cliente muda de ideia toda semana e precisa editar as regras sozinho. O editor visual gera JSON, que vira expressões CEL: sem laços, com verificação de tipos e limite de custo. Scripts (Lua/JS) foram descartados por segurança. Cada publicação é uma versão imutável, e cada decisão grava a versão usada. |
| **Row-Level Security** no Postgres | O tenant vem da chave ou da sessão, nunca do cliente, e toda query filtra por ele. A RLS é a segunda barreira: um bug no código não vaza dados de outro cliente. A API recusa subir se o usuário do banco escapar da RLS. |
| **Fail-closed** | Bureau fora do ar, timeout, política que não compila, erro no antifraude, score fora da faixa: tudo vira `negado`, nunca `aprovado`. Variável de ambiente esquecida produz o estado seguro. |
| **HMAC na API pública** | O Bearer leva só o id da chave. O segredo nunca trafega: ele assina `timestamp.idempotency_key.corpo`. Isso prova a posse e amarra a Idempotency-Key ao corpo assinado. |
| **Dinheiro em centavos** (`int64`) | Nada de `float` em valor guardado ou devolvido. Taxa em pontos-base, parcela pela tabela Price em `big.Rat` com arredondamento definido e testado, tetos contra estouro. |

## Como rodar

Pré-requisitos: Docker, Go 1.26+ e Node 24 (para o painel em modo dev e o e2e).

```bash
cp .env.example .env        # preencha as senhas e gere as chaves com: openssl rand -hex 32
                            # para o painel por http local: COOKIE_INSEGURO_DEV=on e SEED_DEMO=on
docker compose up -d --build
```

- API: `http://127.0.0.1:58080` (direto) ou pelo nginx em `http://127.0.0.1:55173/v1/...`
- Painel: `http://127.0.0.1:55173`

**Semente** (cria o tenant demo, o admin com TOTP, a política modelo publicada e uma chave de API; só fora de produção):

```bash
set -a; . ./.env; set +a
DATABASE_URL="postgres://motor_api:${POSTGRES_APP_PASSWORD}@127.0.0.1:55432/motor?sslmode=disable" \
  SEED_DEMO=on go run ./cmd/semente
```

Ela imprime o e-mail e a senha do admin, o segredo TOTP (cadastre num app autenticador ou use
`go run scripts/totp.go <segredo>`), o id da chave de API e o segredo. Rodar de novo troca a senha e cria outra chave.

**Uma consulta pela linha de comando:**

```bash
MOTOR_CHAVE=ck_live_... MOTOR_SEGREDO=... scripts/consulta-exemplo.sh 000.000.004-34 1000000 24
MOTOR_CHAVE=ck_live_... MOTOR_SEGREDO=... scripts/consulta-exemplo.sh 000.000.004-34 limite   # sem pedido
```

**Consulta de limite (pré-aprovação):** o `pedido` é opcional. Sem ele, quem passa nos cortes e cai numa faixa que
aprova recebe `pre_aprovado` (nunca `aprovado`), `oferta: null` e o limite com as parcelas nos prazos ofertados da
política (aba Oferta do painel; padrão 12/24/36). Pedido parcial (só valor ou só prazo) é 400 no campo que faltou.
Com pedido, o `limite` vem junto de `aprovado` e `contraproposta`; negado e documento inválido trazem `limite: null`.

```json
{ "documento": "000.000.004-34", "finalidade": "concessao_credito", "dados": { "renda": 500000 } }
```

```json
{
  "status": "pre_aprovado",
  "oferta": null,
  "pedido": null,
  "limite": {
    "valor_maximo": 3000000,
    "taxa_mensal_bps": 290,
    "simulacoes": [{ "prazo": 12, "parcela": 299590 }, { "prazo": 24, "parcela": 175239 }, { "prazo": 36, "parcela": 135369 }]
  }
}
```

**Postman:** importe `postman/motor-credito.postman_collection.json` e `postman/local.postman_environment.json`,
preencha `chave_id`, `chave_segredo` e `senha`, e ponha o código do autenticador em `totp` antes do 2FA. A coleção
assina sozinha cada requisição e cobre aprovado, contraproposta, pré-aprovação (só dados), negado, falecido, CPF inválido, CNPJ real,
idempotência e o painel. `make postman` roda a semente e valida a coleção inteira com newman.

Documentos de teste do bureau simulado: [`internal/bureau/README.md`](internal/bureau/README.md).

**Testes:**

```bash
make lint      # go vet, gofmt e ESLint
make test      # Go (inclui integração com Postgres real via testcontainers: precisa de Docker) + Vitest
make test-curto  # só os testes Go que não precisam de Docker
make e2e       # Playwright contra a pilha do compose: entra com TOTP, publica uma faixa e confere a versão na consulta
```

**Desenvolvimento:** `make dev` sobe o Postgres, a API em `:8080` e o painel Vite em `:5179`.

**Retenção LGPD:** `docker compose --profile retencao run --rm retencao`. Para agendar, uma linha no cron do host:

```
10 3 * * *  cd /caminho/motor-credito && docker compose --profile retencao run --rm retencao
```

## Segurança e LGPD

- **Documento:** CPF/CNPJ nunca em texto puro. Fica como HMAC (busca e velocidade) e cifrado com AES-256-GCM,
  com o id da consulta como AAD. O log só mostra o documento mascarado. A busca no painel vai no corpo de um POST,
  nunca na URL.
- **API pública:** HMAC com timestamp de ±5 min, Idempotency-Key reservada com o hash do corpo antes de decidir
  (reenvio devolve a mesma decisão; corpo diferente dá 409), limite de requisições e lista de IPs opcional por chave.
- **Painel:** senha argon2id e TOTP obrigatório, com trava após 5 falhas. Limite de login por IP, sessão de 12 h
  que cai após 30 min sem uso, cookie httpOnly + SameSite=Strict + CSRF, papéis admin, analista e auditor, CSP no nginx.
- **Isolamento:** tenant em toda tabela e RLS. Há teste de que o cliente A recebe 404 ao pedir uma consulta do B.
- **Auditoria:** tabela só de inserção para o usuário da aplicação, com hash encadeado por tenant. Registra decisões,
  publicações de política, chaves, equipe, acessos ao painel e retenção. O painel confere a integridade da cadeia.
- **Retenção:** depois de `RETENCAO_DIAS` (padrão 5 anos), o job apaga o documento cifrado, o HMAC e a
  Idempotency-Key, e mantém só o agregado (status, score, faixa, valores, versão da política).
- **Finalidade e base legal** registradas em cada consulta (proteção ao crédito, LGPD art. 7º, X).
- **IP real** atrás do nginx só a partir de `PROXIES_CONFIAVEIS`.
- CI com `govulncheck`, `gosec`, `npm audit` e `gitleaks`.

## De onde vêm os dados

Cada resposta diz a origem de cada parte em `fontes` (e o painel mostra um selo com texto por fonte):

| Parte | CPF | CNPJ |
|---|---|---|
| Cadastro (situação) | **Real** com `BUREAU_CPF=serpro-trial`/`serpro` (Receita via Serpro; exige `dados.nascimento`, senão `nao_verificado`); simulado por padrão | **Real** com `BUREAU_CNPJ=brasilapi` (Receita via BrasilAPI; padrão do compose); simulado no código |
| Score | Simulado | **Cadastral** com `brasilapi`: 0–1000 a partir de dados públicos, com cada fator em `score_fatores` (tabela em `internal/bureau/README.md`); simulado sem ela |
| Restrições, protestos, dívida, renda | Simulado | Simulado |

A resposta mostra em `cadastro` só a situação para CPF — nome e nascimento vindos da fonte são dado pessoal e
nunca saem (servem só ao antifraude e ao corte de idade). Para CNPJ mostra também razão social e abertura.

**Ligar o Serpro trial** (CPFs fictícios do Serpro, cota mínima — por isso o padrão é `simulado`):

```sh
export BUREAU_CPF=serpro-trial BUREAU_CNPJ=brasilapi
docker compose up -d --build api
# depois consulte, por exemplo, o CPF 404.428.201-35 com "dados": {"nascimento": "1970-11-14"}
```

URL e chave do trial são públicas e entram como padrão só nesse modo. Produção: `BUREAU_CPF=serpro` com
`SERPRO_URL` e `SERPRO_CHAVE` (sem as duas, a API não sobe). Valor desconhecido em `BUREAU_CPF`/`BUREAU_CNPJ`
também impede subir. Os CPFs de teste do Serpro estão em `internal/bureau/README.md`.

## Simulador de integração

No painel, **Simulador de integração** (só admin) faz uma chamada real da `POST /v1/consultas` com uma chave do
tenant: o servidor assina com o segredo da chave (que não sai dele) e roda a requisição pelo mesmo router da `/v1`,
com o IP do usuário do painel (lista de IPs e limite valem). A tela mostra a requisição com a `X-Assinatura` real,
a resposta e o mesmo pedido em curl, Node e Python, que leem o segredo de `MOTOR_SEGREDO`. Na auditoria fica
`simulador.consulta` com o usuário que disparou.

## Limitações (honestas)

- **Restrições, protestos, dívida e renda são simulados.** Determinísticos, derivados do documento. Não existe
  fonte gratuita disso, nem score de crédito comportamental gratuito (ver "De onde vêm os dados").
- **O Serpro de verdade é pago.** O modo `serpro-trial` usa o ambiente de testes público: só CPFs fictícios e
  cota mínima (estourada = `BUREAU_INDISPONIVEL`). Produção (`BUREAU_CPF=serpro`) pede contrato com o Serpro.
- **O CNPJ real vem da BrasilAPI, que não tem SLA.** Fora do ar ou limitando (429), toda consulta de CNPJ é negada
  com `BUREAU_INDISPONIVEL`. CNPJ alfanumérico ainda não é aceito por ela. No código o padrão de `BUREAU_CNPJ` é
  `simulado` (sem rede); o `docker compose` de demonstração liga `brasilapi` mesmo com a variável vazia.
- **O score cadastral de CNPJ mede robustez de cadastro, não comportamento de pagamento.** É explicável e
  determinístico, mas não substitui um bureau.
- A velocidade entre clientes sai do Postgres (sem Redis). Compartilhar esse sinal entre clientes precisaria estar
  em contrato antes de virar produto.
- Os limites de requisição e de login ficam na memória do processo: com mais de uma instância, precisam ir para
  Redis ou Postgres.
- Virar produto pode exigir contrato com bureaus e, conforme o modelo, autorização do Bacen.

## Próximos passos

- Fila de análise manual (`em_analise`) com webhook.
- Simular uma política nova sobre consultas antigas antes de publicar.
- Bureaus reais (Serasa, Boa Vista, SCR) atrás da interface `Bureau`.
- Provedores terceirizados por cliente: cada tenant escolhe e configura as próprias fontes e credenciais.

## Como foi construído

Projeto pessoal, feito do zero para estudar Go num problema real. **O código foi escrito por
agentes de IA (Claude Code); a arquitetura, as decisões e a condução foram minhas.** O método é o que eu quis
testar:

1. **Desenho antes de código.** Requisitos, decisões e trade-offs discutidos e escritos em
   [`docs/superpowers/specs/`](docs/superpowers/specs/) antes da primeira linha.
2. **Contrato para paralelizar.** Um plano com interfaces Go exatas, dono de cada pasta e ondas de trabalho
   ([`docs/superpowers/plans/`](docs/superpowers/plans/)). Quatro agentes trabalharam ao mesmo tempo, cada um no
   próprio worktree, sem arquivo em comum, e os merges entraram sem conflito.
3. **Quem escreve não confere.** Três conferências por agentes novos, sem histórico, acharam 23 problemas
   (segurança, estouro numérico, corrida de idempotência). Outro agente corrigiu, e uma conferência final verificou
   no código. Relatórios em [`docs/conferencias/`](docs/conferencias/).
4. **Teste só vale se pega o erro.** Regras de dinheiro, motor, antifraude, HMAC e isolamento têm mutação manual
   provada: a regra é quebrada de propósito e o teste precisa ficar vermelho.

Em números: 85 commits, ~7,8 mil linhas de Go + ~5,6 mil de teste, 129 funções de teste em Go (com integração em
Postgres real), 117 testes no painel, e2e com Playwright e a coleção Postman validada com newman.

## Licença

[MIT](LICENSE) © Romero Almeida
