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
