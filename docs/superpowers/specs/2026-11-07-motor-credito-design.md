# Motor de Crédito — Desenho

**Data:** 07/11/2026 · **Estado:** desenho aprovado em conversa, aguardando revisão do documento · **Atualizado em 07/11/2026** (após as conferências): §4 e §7 refletem a assinatura nova, o CNPJ via BrasilAPI, a retenção e a auditoria de acesso.

## 1. Objetivo

Uma empresa cliente envia um CPF ou CNPJ (mais dados opcionais) e recebe, em uma chamada síncrona, uma decisão de crédito precificada: score, faixa de risco, decisão, oferta (valor, prazo, taxa, parcela), motivos e resultado do antifraude.

Cada cliente define a própria política de crédito num painel, partindo de um modelo pronto, e pode mudá-la a qualquer momento.

**Trajetória:** começa como projeto de portfólio público no GitHub, com fontes de dados simuladas, e é desenhado para virar produto: bureaus reais entram atrás de uma interface, sem mexer no motor.

## 2. Decisões tomadas

| Tema | Decisão | Porquê |
|---|---|---|
| Linguagem do backend | Go | Binário pequeno, poucas dependências, tipagem forte, concorrência nativa para consultar fontes em paralelo |
| Painel | React + TypeScript + Vite | Stack que o autor domina |
| Banco | PostgreSQL (só ele) | Transação única para decisão e auditoria; Row-Level Security para isolar clientes |
| Arquitetura | Monólito modular | Um desenvolvedor; microserviços trariam rede autenticada entre serviços, consistência distribuída e vários deploys sem ganho agora. Saída preservada (ver §3) |
| Quem define a política | Cada cliente, com editor flexível, partindo de modelo pronto | O cliente muda de ideia de um dia para o outro ("score até 100 libera taxa X" hoje, amanhã não) |
| Execução das regras | Editor visual → JSON → expressões CEL (`cel-go`) | Flexível sem executar código arbitrário; CEL não tem laços, tem verificação de tipos e limite de custo. Scripts (Lua/JS) foram descartados por segurança |
| Tipo de crédito | Núcleo genérico + modelo inicial de empréstimo PF | Serve qualquer cliente; o modelo PF dá uma demonstração concreta |
| Entrada | CPF/CNPJ obrigatório + dados opcionais | Integração simples; cada dado extra liga mais checagens antifraude |
| Resultados | aprovado · negado · contraproposta · documento_invalido (+ `em_analise` reservado) | Contraproposta é síncrona e é o que mais vende; a fila manual fica para a v2 |

## 3. Arquitetura

```
motor-credito/
├── cmd/api/              → binário único
├── internal/
│   ├── consulta/         → orquestra a consulta e monta a resposta
│   ├── bureau/           → interface Bureau + adaptador simulado
│   ├── antifraude/       → checagens que produzem sinais e risco
│   ├── politica/         → modelo JSON → CEL, validação e avaliação (o "motor")
│   ├── consulta/ … repositorio/ → integração (ver plano de agentes)
│   ├── tenant/           → clientes, chaves de API, usuários, papéis
│   └── auditoria/        → registro encadeado, só inserção
├── web/                  → painel React
└── migrations/
```

**Regras de fronteira (o que preserva a saída para microserviços):**
- Um módulo só fala com outro por interface Go; nunca acessa as tabelas do vizinho.
- Cada módulo tem as próprias tabelas, com prefixo (`antifraude_*`, `politica_*`, …).
- Extrair um serviço = trocar a implementação da interface por um cliente HTTP/gRPC.
- Sinais para separar: antifraude precisando escalar sozinho; bureau lento derrubando consultas; segundo time.

**Entradas:**
- `POST /v1/consultas`: API para clientes, autenticada por chave de API + HMAC.
- `/painel/*`: API do painel, com sessão em cookie e papéis.

**Fluxo de uma consulta:**
1. Valida o formato e o dígito verificador do CPF/CNPJ. Se inválido, devolve `documento_invalido`.
2. Bureau e antifraude rodam em paralelo, com tempo máximo de 2s.
3. Carrega a versão publicada da política do cliente.
4. O motor executa: regras de corte → faixas de preço → contraproposta.
5. Grava a decisão e a auditoria na mesma transação.
6. Responde.

**Infraestrutura:** `docker compose up` (api + postgres + painel). Na publicação, Caddy faz o TLS. Sem Redis: a checagem de velocidade usa consulta indexada no Postgres. *shortcut: a velocidade sai do Postgres; trocar por Redis quando o volume de consultas pedir.*

## 4. Segurança e LGPD

**API dos clientes**
- A chave tem duas partes: o id público `ck_live_<id>`, que vai em `Authorization: Bearer ck_live_<id>`, e o segredo, que aparece uma vez na criação e **nunca trafega**. O banco guarda o segredo cifrado (AES-256-GCM, subchave própria derivada por HKDF, AAD = id da chave), porque o servidor precisa dele para conferir a assinatura. Dá para girar e revogar. *(Atualizado em 07/11 — conferência de auth, achado 2: antes o Bearer levava a chave inteira e ela mesma era a chave do HMAC, o que não provava posse.)*
- `X-Assinatura = hex(HMAC-SHA256(segredo, X-Timestamp + "." + Idempotency-Key + "." + corpo cru))` (`tenant.Assinar`). A `Idempotency-Key` entra na assinatura: não dá para reaproveitar um corpo assinado com outra chave de idempotência. Timestamp fora de ±5 min é recusado.
- `Idempotency-Key`: reenvio não duplica registro nem cobrança. A chave é reservada com o hash do corpo antes de decidir (trava por tenant + chave), então duas requisições simultâneas com corpos diferentes nunca recebem a decisão uma da outra (409).
- Limite de requisições por cliente e lista de IPs permitidos opcional.
- IP do cliente: atrás do nginx, `X-Forwarded-For` só vale se a conexão vier de um proxy listado em `PROXIES_CONFIAVEIS` (vazio = ninguém; usa o endereço da conexão). Serve ao limite de login por IP, à lista de IPs da chave e à auditoria.

**Painel**
- Senha com argon2id (teto de cálculos simultâneos) + 2FA TOTP obrigatório, com trava persistente após 5 falhas seguidas e limite de tentativas de login por IP.
- Sessão expira em 12 h ou depois de 30 min sem uso.
- Cookie httpOnly, Secure, SameSite=Strict + token CSRF.
- Papéis: `admin` (política, chaves, equipe), `analista` (consultas), `auditor` (só auditoria).
- Toda publicação de política exige confirmação e grava quem publicou e quando.
- Cadastro por convite responde o mesmo 404 para convite inválido e para e-mail que já tem conta (não revela quem tem conta).

**Isolamento entre clientes**
- `tenant_id` em toda tabela + Row-Level Security no Postgres como segunda barreira.
- Teste obrigatório: o cliente A pede uma consulta do cliente B e recebe 404.

**LGPD**
- CPF/CNPJ guardado de duas formas: HMAC com chave secreta (busca e velocidade) e cifrado com AES-256-GCM com AAD = id da consulta (exibição ao dono da consulta; o cifrado copiado para outra linha não decifra, e o painel mostra `***`). Nunca em texto puro. Log mostra mascarado (`***.456.789-**`).
- Cada consulta declara a finalidade (`concessao_credito`) e a base legal (proteção ao crédito, art. 7º, X). Ambas ficam registradas.
- Retenção configurável, padrão de 5 anos. Depois, um job apaga os dados pessoais e mantém só os agregados. *(Implementado:* `cmd/retencao`, `RETENCAO_DIAS` padrão 1825, inteiro ≥ 1. Apaga `doc_cifrado`, `doc_hash` — que permitiria reidentificar pela busca — e a `Idempotency-Key`; mantém status, score, faixa, valores, política e versão; marca `dados_apagados_em`; apaga eventos de velocidade anteriores ao corte. Roda com o usuário da aplicação e a RLS valendo, é idempotente e registra `lgpd.retencao_aplicada` na auditoria de cada tenant. Agendamento: `docker compose --profile retencao run --rm retencao` no cron do host.*)*
- Fonte de CNPJ: com `BUREAU_CNPJ=brasilapi`, o cadastro (razão social, situação, abertura) vem da Receita via BrasilAPI e o resto do simulado; CPF nunca sai para a rede. Só dado público de empresa; o quadro de sócios é descartado.
- Endpoint de explicação da decisão para atender o titular.

**Auditoria**
- Tabela só de inserção: o usuário do banco que a aplicação usa não tem `UPDATE`/`DELETE` nela.
- Cada registro guarda o hash do anterior, então alteração manual é detectável.
- Eventos: consulta decidida, política publicada, chaves (criar, girar, revogar, IPs), equipe (convite, aceite, remoção, troca de papel), acesso ao painel (login ok, senha errada, 2FA errado, saída) e retenção LGPD aplicada.

**Cadeia de dependências e segredos**
- Segredos só por variável de ambiente; `.env.example` sem valores.
- CI: `govulncheck`, `gosec`, `npm audit`, `gitleaks`. Dependabot ligado.

## 5. Motor e política

**Contexto disponível para as regras CEL**

```
score                  int 0–1000 (bureau)
bureau.restricoes      int
bureau.protestos       int
bureau.divida_total    int (centavos)
renda                  int (centavos), opcional
idade                  int, opcional
pedido.valor           int (centavos)
pedido.prazo           int (meses)
fraude.risco           int 0–100
fraude.sinais          list(string)
```

Os campos opcionais ausentes são declarados no ambiente CEL e testados com `has()`. Uma regra que usa campo ausente sem `has()` é recusada na publicação.

**Estrutura da política (três blocos, nesta ordem)**
1. **Regras de corte:** uma lista ordenada de expressões booleanas. A primeira verdadeira nega, com seu código de motivo.
2. **Faixas de preço:** linhas com condição → taxa mensal e limite. O limite pode ser um valor fixo ou um múltiplo da renda. A primeira linha que casar vale. Uma linha pode ser "nega".
3. **Contraproposta:** se `pedido.valor` passa do limite da faixa, oferece o limite. Opcionalmente com prazo e taxa ajustados por regra. Desligada, o pedido acima do limite é negado.

**Regras fixas, fora da política:** documento com dígito inválido e CPF de pessoa falecida dão sempre `documento_invalido`.

**Ciclo de vida da política**
- Rascunho editável → testar com CPF de exemplo → publicar (gera versão N+1, imutável).
- Cada decisão grava `politica.id` e `versao`.
- Histórico com diferença entre versões; reverter = publicar uma cópia da versão antiga como nova versão.
- Ao publicar: compila todo o CEL, verifica tipos e recusa expressão acima do limite de custo ou de tamanho.

**Modelo inicial "Empréstimo PF"**
- Corte: `fraude.risco >= 70`, `bureau.restricoes > 2`, `has(idade) && idade < 18`.
- Faixas: 0–300 nega · 301–600 → 5,9% a.m., até 3× renda · 601–1000 → 2,9% a.m., até 8× renda (sem renda: limite fixo).
- Contraproposta ligada.

**Dinheiro:** sempre em centavos (`int64`). Taxa em pontos-base. Parcela pela tabela Price, com arredondamento definido e testado.

**Falha de bureau** (erro ou tempo acima de 2s): `negado` com motivo `BUREAU_INDISPONIVEL`. Nunca aprova sem dados.

## 6. Antifraude

O antifraude informa e não decide: devolve `risco` (soma dos pesos, limitada a 100), `sinais`, `checagens_executadas` e `checagens_puladas`. Quem decide é a política.

| Checagem | Precisa de | Sinal |
|---|---|---|
| Situação do documento | doc | `cpf_irregular`, `cnpj_inativo` |
| Velocidade (todos os clientes, por HMAC do doc) | doc | `velocidade_alta` |
| Nome × bureau | nome | `nome_divergente` |
| Nascimento × bureau | nascimento | `nascimento_divergente` |
| E-mail descartável | email | `email_descartavel` |
| IP | ip | `ip_estrangeiro`, `ip_datacenter` |
| Dispositivo em vários documentos | device_id | `dispositivo_multiplos_cpfs` |

A velocidade entre clientes não revela qual cliente consultou o documento.

## 7. Contrato da API

**Requisição** `POST /v1/consultas`

```json
{
  "documento": "123.456.789-09",
  "finalidade": "concessao_credito",
  "pedido": { "valor": 1000000, "prazo": 24 },
  "dados": {
    "nome": "...", "nascimento": "1990-05-01", "renda": 500000,
    "email": "...", "telefone": "...", "ip": "...", "device_id": "..."
  }
}
```

Cabeçalhos: `Authorization: Bearer ck_live_<id>` (só o id; o segredo nunca vai em cabeçalho), `X-Timestamp` (unix, segundos), `Idempotency-Key` e `X-Assinatura = hex(HMAC-SHA256(segredo, ts + "." + idempotency_key + "." + corpo))`. Ver §4.

**Resposta**

```json
{
  "id": "cons_01JC...",
  "status": "contraproposta",
  "score": 642,
  "faixa_risco": "B",
  "oferta": { "valor": 600000, "prazo": 24, "taxa_mensal_bps": 290, "parcela": 35048 },
  "pedido": { "valor": 1000000, "prazo": 24 },
  "motivos": [{ "codigo": "LIMITE_FAIXA", "texto": "Valor pedido acima do limite para o score" }],
  "antifraude": {
    "risco": 15,
    "sinais": ["email_descartavel"],
    "checagens_executadas": ["documento", "velocidade", "email"],
    "checagens_puladas": ["ip", "dispositivo", "nome", "nascimento"]
  },
  "politica": { "id": "pol_...", "versao": 7 },
  "criado_em": "2026-11-07T14:02:11Z"
}
```

`status` ∈ `aprovado | negado | contraproposta | documento_invalido | em_analise` (este último reservado para a v2). Valores em centavos.

**Erros:** 400 (corpo inválido), 401 (chave ou assinatura), 409 (mesma `Idempotency-Key` com corpo diferente), 429 (limite), 503 nunca. Indisponibilidade de bureau (inclusive a BrasilAPI fora do ar ou com 429) vira decisão `negado` com `BUREAU_INDISPONIVEL`, não erro. CNPJ que a Receita não conhece (404 na BrasilAPI) vira `documento_invalido`.

Contrato publicado em OpenAPI, com exemplos em `curl`, Node e Python.

## 8. Painel

**Telas da v1:** Entrar (senha + 2FA) · Visão geral (consultas do dia, aprovação, distribuição de score, sinais frequentes) · Consultas (lista com filtros + detalhe com cada regra avaliada e a versão da política) · Política (abas Corte / Faixas / Contraproposta; rascunho, testar, publicar, histórico com diferenças) · Chaves de API · Equipe.

**Direção visual** (referência: `docs/referencias/ui-pinterest-695032155015577928.jpg`):
- Fundo cinza muito claro; cards brancos com cantos bem arredondados (~20px) e sombra suave e difusa.
- Elementos de destaque (botões primários, chips ativos, ícones redondos) em quase-preto.
- Cor só como sinal, em bolinhas e chips: verde = aprovado, vermelho = negado, roxo = contraproposta, âmbar = alerta de fraude.
- Tipografia sans grotesca, muito espaço em branco, menus flutuantes em card escuro.
- Precisa funcionar no celular e ter contraste acessível; não usar a cor como único sinal (sempre com texto).

## 9. Testes

- **Motor/política:** testes em tabela (contexto + política → decisão), nos limites exatos de cada faixa (300/301), contraproposta e regras fixas.
- **Dinheiro:** parcela Price conferida contra valores de referência calculados à parte.
- **Segurança:** assinatura inválida, timestamp vencido, chave revogada, idempotência com corpo diferente, isolamento A×B, CEL acima do limite recusada.
- **Integração:** fluxo completo com Postgres real (testcontainers).
- **Painel:** Vitest no editor; um teste ponta a ponta com Playwright (editar → publicar → consultar → ver a versão nova na decisão).
- Tudo no CI do GitHub, em cada PR.

## 10. Fora da v1

Fila de análise manual + webhook · simular uma política nova sobre consultas antigas · bureaus reais (Serasa, Boa Vista, SCR) · score próprio por machine learning · cobrança e planos · Redis · microserviços.

## 11. Ordem de construção

1. Base: repositório, CI, Postgres, migrations, clientes, chaves de API + HMAC.
2. Consulta de ponta a ponta com bureau simulado e política fixa.
3. Política versionada (JSON → CEL) + motor.
4. Antifraude.
5. Painel: entrar, consultas, política, chaves.
6. Auditoria encadeada, retenção LGPD e README com o porquê das decisões.

## 12. Riscos conhecidos

- **Regulatório:** virar produto pode exigir contrato com bureaus e, conforme o modelo, autorização do Bacen. O portfólio não depende disso.
- **Bureau simulado:** dados falsos podem esconder casos reais (homônimos, CPF com dados incompletos). O adaptador simulado deve ter perfis variados e campos ausentes.
- **Velocidade entre clientes:** compartilhar sinal entre clientes precisa estar previsto em contrato e na política de privacidade antes de virar produto.
