# bureau

Contrato `Bureau` e o adaptador simulado `NovoSimulado(atraso)`.

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
os testes passam a URL de um `httptest`). Só traz **cadastro**: `Nome` (razão social),
`Situacao` e `Abertura` (`data_inicio_atividade`, AAAA-MM-DD). Não existe score grátis:
score, restrições, protestos, dívida e renda continuam vindo do simulado.

Como ligar: `bureau.NovoComposto(brasilapi.Novo(""), bureau.NovoSimulado(atraso))`.
CNPJ pega o cadastro da BrasilAPI e o resto do simulado; CPF vai só para o simulado.

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
Fixture `brasilapi/testdata/cnpj_00000000000191.json`: resposta real do Banco do Brasil, sem `qsa`.
