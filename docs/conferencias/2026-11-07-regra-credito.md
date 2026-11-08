# Conferência da regra de crédito — 07/11/2026 (agente novo, somente leitura, mutações em cópia)

Veredito: nenhum caminho em que erro/timeout/ctx/panic/dado ausente vira `aprovado`. Achados:

1. **[médio] Parcela estoura int64 e sai negativa com `aprovado`.** `internal/dinheiro/dinheiro.go:38` (`q.Int64()` sem checar). Sem teto em `pedido.valor` (`internal/consulta/tipos.go:110`) nem `limite_fixo` (`internal/politica/compilar.go:158-171`). Repro: limite_fixo=MaxInt64, pedido 9e18, prazo 1, taxa 290 → parcela negativa. Correção: teto (ex.: 1e13 centavos) em pedido.valor e limite_fixo; `Parcela` devolve erro quando `!q.IsInt64()` (ajustar chamadores: erro → negado).
2. **[baixo] Soma de taxas estoura.** `internal/politica/avaliar.go:82`. Correção: teto em taxa_bps e acrescimo_taxa_bps (≤ 100000) em validarFaixa/validarContraproposta.
3. **[baixo] 2^63 no editor visual vira MinInt64 em amd64.** `internal/politica/compilar.go:431` usa `v > math.MaxInt64`; trocar por `v >= math.MaxInt64` (float). Teste fraco: a mutação passou verde.
4. **[baixo] Perda de precisão em Condicao.Valor > 2^53.** `internal/politica/politica.go:21`. Correção: recusar inteiros acima de 2^53 (ou json.Number).
5. **[médio] Corrida na idempotência com corpo diferente.** `internal/repositorio/consultas.go:46-50` devolve decisão gravada sem comparar hash; `internal/httpapi/consultas.go:77-85` só detecta depois; falha de GuardarIdempotencia só vai para log. Correção: guardar `hash_corpo` junto da consulta (ou reservar a chave com hash ANTES de Decidir, na mesma trava) e devolver ErrIdempotenciaConflito quando divergir; teste concorrente. Considerar índice único `(tenant_id, idempotency_key)` em migration nova `0002_*.sql`.
6. **Teste fraco:** guarda de overflow da renda `internal/politica/avaliar.go:95` (mutação `MaxInt64/mult` → `MaxInt64` passou).
7. **Teste fraco:** teto do múltiplo `internal/politica/compilar.go:168` (mutação `case false:` passou).
8. **Teste fraco [médio]:** nascimento do bureau prevalece sobre o informado `internal/consulta/consulta.go:130-133` (inverter passou → menor escapa do corte MENOR_IDADE). Também: nascimento do bureau inválido não cai para o informado.
9. **Teste fraco:** renda informada prevalece sobre a do bureau `internal/consulta/consulta.go:127-129`.
10. **Teste fraco:** antifraude dentro do timeout de 2s `internal/consulta/consulta.go:90` (trocar ctxFontes por ctx passou).
11. CPF na query string do painel — tratado pelo E2.
12. **[a confirmar]** score do bureau sem validação de faixa `internal/consulta/consulta.go:86`: score fora de 0..1000 → tratar como BUREAU_INDISPONIVEL (fail-closed).
