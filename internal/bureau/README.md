# bureau

Contrato `Bureau` (e o opcional `ComNascimento`), o adaptador simulado `NovoSimulado(atraso)`,
o composto `NovoComposto(cpf, cnpj, base)` e o score cadastral de CNPJ.

O simulado é determinístico: o mesmo documento devolve sempre o mesmo resultado.
Documentos fora da tabela abaixo ganham um perfil derivado do SHA-256 do documento,
com situações variadas e campos ausentes (sem nome, sem nascimento, sem renda),
para não esconder casos reais (desenho §12).

O `atraso` é aplicado a toda consulta e respeita o `ctx`: cancelado ou vencido,
a consulta volta na hora com erro que casa com `ErrIndisponivel` e com a causa
(`context.DeadlineExceeded` / `context.Canceled`).

## Documentos-fixture

Todos têm dígitos verificadores válidos e são obviamente de teste: CPFs com oito zeros
à frente e CNPJs alfanuméricos começando por `EXEMPLO`. Constantes em `simulado.go`.

| Documento | Constante | Simula |
|---|---|---|
| `000.000.001-91` | `CPFFalecido` | situação `falecido` (E trata como `documento_invalido`) |
| `000.000.002-72` | `CPFIrregular` | situação `irregular`, 2 restrições |
| `000.000.003-53` | `CPFScoreBaixo` | score 150, restrições e protestos |
| `000.000.004-34` | `CPFScoreAlto` | score 920, renda R$ 12.000,00 |
| `000.000.005-15` | `CPFNomeConhecido` | nome `JOANA PEREIRA DOS SANTOS`, nascimento `1985-04-12`; mande outro sobrenome para `nome_divergente` ou outra data para `nascimento_divergente` |
| `000.000.006-04` | `CPFIndisponivel` | devolve `ErrIndisponivel` na hora |
| `000.000.007-87` | `CPFLento` | demora 30 s (acima do timeout de 2 s) — vira `ErrIndisponivel` quando o `ctx` vence |
| `000.000.008-68` | `CPFCamposAusentes` | regular, sem nome, sem nascimento, sem renda |
| `EX.EMP.LO0/0001-54` (`EXEMPLO0000154`) | `CNPJInativo` | CNPJ alfanumérico com situação `inativo` |
| `EX.EMP.LO0/0002-35` (`EXEMPLO0000235`) | `CNPJRegular` | CNPJ alfanumérico regular, score 810 |

Documento com dígito errado volta `documento.ErrInvalido`.

## CNPJ real via BrasilAPI (`brasilapi` + `NovoComposto`)

`brasilapi.Novo(base)` consulta o cadastro público da Receita em
`GET https://brasilapi.com.br/api/cnpj/v1/{cnpj}` (base `""` = `brasilapi.URLPadrao`;
os testes passam a URL de um `httptest`). Traz o **cadastro**: `Nome` (razão social),
`Situacao`, `Abertura` (`data_inicio_atividade`, AAAA-MM-DD) e `Empresa` (capital social em
centavos, `codigo_porte`, `opcao_pelo_simples`, `opcao_pelo_mei`, `codigo_natureza_juridica` e a
**contagem** do `qsa` — os dados dos sócios não são lidos). Com `Empresa`, o composto troca o score
do CNPJ pelo **score cadastral** (abaixo). Restrições, protestos, dívida e renda continuam do simulado:
não existe fonte grátis disso.

Como ligar: `bureau.NovoComposto(nil, brasilapi.Novo(""), bureau.NovoSimulado(atraso))`
(`BUREAU_CNPJ=brasilapi` no `cmd/api`).

| Resposta da BrasilAPI | Resultado |
|---|---|
| 200, `descricao_situacao_cadastral` = `ATIVA` | `regular` |
| 200, `BAIXADA` / `INAPTA` / `SUSPENSA` / `NULA` | `inativo` (sinal `cnpj_inativo`) |
| 404 | `inexistente` — a orquestração trata como `documento_invalido`; o simulado nem é chamado |
| 429, 5xx, 400, timeout do `ctx`, JSON inválido, corpo > 1 MB, situação desconhecida, data malformada | `ErrIndisponivel` (fail-closed; nunca cai para o cadastro do simulado) |

Limites que é preciso aceitar ao ligar:

- **Sem SLA.** Serviço comunitário e gratuito; fora do ar = toda consulta de CNPJ nega com
  `BUREAU_INDISPONIVEL`. O timeout é o do `ctx` da orquestração (2 s).
- **Limite de requisições** da BrasilAPI (não documentado em número; responde 429 e às vezes
  bloqueia por IP). Volume alto pede cache ou um fornecedor pago.
- **Dado público de empresa**: o mesmo da Receita, com atraso de dias. O quadro de sócios
  vem na resposta e é descartado (não é lido nem guardado).
- CNPJ **alfanumérico** (emitido desde jul/2026) ainda não é aceito pela BrasilAPI: volta 400,
  e o composto devolve `ErrIndisponivel`. Os fixtures `EXEMPLO...` só funcionam com o simulado puro.
- O erro devolvido não leva a URL (que contém o CNPJ), porque vai para o log.

Teste ao vivo (pulado por padrão): `BRASILAPI_AO_VIVO=on go test ./internal/bureau/brasilapi -run AoVivo`.

## Score cadastral de CNPJ (`ScoreCadastral`)

Função pura em `scorecadastral.go`: 0–1000, soma de sete fatores, cada um devolve código e pontos
(`Resultado.ScoreFatores`, que a API devolve em `score_fatores`). Mesma empresa e mesmo dia = mesmo score.
Os topos somam exatamente 1000 e nenhum fator é negativo.

| Fator | Faixas (código → pontos) |
|---|---|
| Situação | `SITUACAO_ATIVA` 250 · `SITUACAO_INATIVA` 0 |
| Tempo de empresa (anos completos desde a abertura) | `TEMPO_EMPRESA_MENOS_1A` 0 · `TEMPO_EMPRESA_1A_3A` (1–2) 80 · `TEMPO_EMPRESA_3A_5A` (3–4) 140 · `TEMPO_EMPRESA_5A_MAIS` 200 · `TEMPO_EMPRESA_DESCONHECIDO` (sem data, malformada ou futura) 0 |
| Capital social | `CAPITAL_ATE_10MIL` (< R$ 10 mil) 0 · `CAPITAL_10MIL_100MIL` 50 · `CAPITAL_100MIL_1MI` 100 · `CAPITAL_1MI_MAIS` (≥ R$ 1 mi) 150 |
| Porte (`codigo_porte`) | `PORTE_MICRO` (1) 40 · `PORTE_PEQUENO` (3) 70 · `PORTE_DEMAIS` (5) 100 · `PORTE_NAO_INFORMADO` 0 |
| Regime | `REGIME_MEI` 0 · `REGIME_SIMPLES` 50 · `REGIME_NORMAL` (fora do Simples) 100 |
| Natureza jurídica | `NATUREZA_SA` (2011, 2038, 2046, 2054) 150 · `NATUREZA_LIMITADA` (2062) 100 · `NATUREZA_INDIVIDUAL` (2135, 2305, 2313) 30 · `NATUREZA_OUTRA` 0 |
| Sócios (contagem do `qsa`) | `SOCIOS_2_MAIS` 50 · `SOCIO_1` 20 · `SOCIOS_NAO_INFORMADO` (0) 0 |

O **CNAE fica de fora**: risco por setor exige base estatística de inadimplência que não temos, e uma lista
feita à mão não se defende numa negativa. Exemplos: Banco do Brasil (fixture, sem `qsa`) = 950; MEI aberto
há 5 meses, R$ 1 mil de capital, 1 sócio = 340. É um score de **robustez cadastral**, não de comportamento de
pagamento: não substitui um bureau de crédito.

## CPF real via Serpro (`serpro` + `NovoComposto`)

`serpro.Novo(url, chave, trial)` consulta `GET {url}/cpf/{cpf}/{nascimento DDMMAAAA}` com
`Authorization: Bearer <chave>` (API Consulta CPF v3). O Serpro **exige o nascimento**; por isso o
adaptador implementa `bureau.ComNascimento` e a orquestração repassa `dados.nascimento`
(`bureau.Consultar(ctx, b, doc, nascimento)`; quem não implementa recebe só o documento).
Só cadastro: situação, nome e nascimento (usados no antifraude e na idade; **nunca** devolvidos na API).
Score, restrições, protestos, dívida e renda do CPF continuam do simulado.

| Resposta do Serpro | Resultado |
|---|---|
| 200, situação `0` regular | `regular` |
| 200, `2` suspensa / `4` pendente de regularização | `irregular` (sinal `cpf_irregular`) |
| 200, `3` titular falecido | `falecido` → `documento_invalido` |
| 200, `5` cancelada por multiplicidade / `8` nula / `9` cancelada de ofício | `inexistente` → `documento_invalido` |
| 206 conteúdo parcial | usa o que veio; campo ausente fica ausente (sem situação = checagem de documento pulada) |
| 422 menor de 18 | devolve o nascimento informado: a idade entra e o corte `MENOR_IDADE` da política nega. Se o nascimento informado diz 18+ (contradiz a Receita), vira `inexistente` → `documento_invalido` |
| 451 menor de 16 | `inexistente` → `documento_invalido` (o Serpro não devolve dado; menor de 16 é absolutamente incapaz de contratar; motivo genérico como o do falecido, desenho §5) |
| 429 (cota), 401/403 (chave), 400, 404, 5xx, timeout do `ctx`, JSON inválido, corpo > 64 KB, situação ausente no 200 ou desconhecida, nascimento malformado | `ErrIndisponivel` (fail-closed) |

**Sem nascimento informado** e Serpro ligado: o Serpro não é chamado, nada é inventado — situação, nome e
nascimento ficam vazios (o do simulado não vale como real), a checagem `documento` é pulada (não pontua) e
`fontes.cadastro` = `nao_verificado`. O erro nunca leva a URL (CPF e nascimento) nem a chave.

`BUREAU_CPF` no `cmd/api`: `simulado` (padrão), `serpro-trial` (ambiente de testes público; `SERPRO_URL` e
`SERPRO_CHAVE` vazias usam `serpro.URLTrial`/`serpro.ChaveTrial`, que são públicas na documentação) ou
`serpro` (produção; sem `SERPRO_URL` **e** `SERPRO_CHAVE` a API recusa subir). Valor desconhecido recusa subir.
CPFs de teste do trial (CPF / situação / nascimento): 40442820135 regular 14/11/1970; 63017285995 regular
26/12/1964; 40532176871 suspensa 22/03/1982; 07691852312 pendente 13/08/2000; 01648527949 cancelada por
multiplicidade 18/02/1996; 98302514705 nula 05/10/1999; 64913872591 cancelada de ofício 22/04/1956;
05137518743 falecido 05/07/1965; 30272095079 menor de 18 (422) 03/02/2009; 20803407009 menor de 16 (451)
16/07/2021; 88373571094 parcial (206) 02/02/2000.

Teste ao vivo (pulado por padrão; gasta a cota): `SERPRO_AO_VIVO=on go test ./internal/bureau/serpro -run AoVivo`.
Fixtures em `serpro/testdata`: respostas reais do trial (dados fictícios) e o 429 de cota estourada.

## Fontes (`Resultado.Fontes`)

Cada adaptador marca a sua parte e o composto junta: `cadastro` (`serpro`, `serpro-trial`, `brasilapi`,
`simulado`, `nao_verificado`), `score` (`cadastral` ou `simulado`) e `restricoes` (sempre `simulado`).
Fixture `brasilapi/testdata/cnpj_00000000000191.json`: resposta real do Banco do Brasil, sem `qsa`.
