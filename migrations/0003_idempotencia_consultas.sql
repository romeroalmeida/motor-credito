-- Idempotência na própria consulta (conferência da regra de crédito, achado 5).
-- O hash do corpo fica junto da decisão: a mesma Idempotency-Key com outro corpo nunca recebe a decisão
-- gravada, nem mesmo na corrida entre duas requisições simultâneas. Linhas antigas ficam com hash nulo e,
-- por isso, qualquer reenvio delas é tratado como conflito (fail-closed).
ALTER TABLE consultas ADD COLUMN idempotency_hash bytea;

-- Segunda barreira contra duas consultas com a mesma chave (a primeira é a trava em GravarConsultaIdempotente)
-- e índice para a busca pela chave, que antes varria a tabela.
CREATE UNIQUE INDEX consultas_tenant_idem ON consultas (tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
