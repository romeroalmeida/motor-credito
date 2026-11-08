-- Retenção LGPD também na tabela idempotencia (chave do cliente, hash do corpo e resposta guardada).
-- A função passa a listar também os tenants com linhas de idempotencia vencidas: um tenant cujas consultas já
-- foram anonimizadas por uma execução anterior (antes desta correção) ainda precisa ter o cache limpo.
CREATE OR REPLACE FUNCTION retencao_tenants_pendentes(p_antes timestamptz)
RETURNS TABLE (tenant_id text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    SELECT c.tenant_id FROM public.consultas c WHERE c.criado_em < p_antes AND c.dados_apagados_em IS NULL
    UNION
    SELECT i.tenant_id FROM public.idempotencia i WHERE i.criado_em < p_antes
$$;

CREATE INDEX idempotencia_retencao ON idempotencia (criado_em);
