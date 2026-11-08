-- Retenção LGPD (desenho §4): o job cmd/retencao roda com o usuário da aplicação (motor_app, sujeito à RLS).
-- Ele precisa saber QUAIS tenants têm consultas vencidas antes de entrar em cada um; esta função devolve só os
-- ids (nada de dado pessoal). O apagamento em si é um UPDATE por tenant, dentro de EmTenant, com a RLS valendo.
CREATE FUNCTION retencao_tenants_pendentes(p_antes timestamptz)
RETURNS TABLE (tenant_id text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    SELECT DISTINCT c.tenant_id FROM public.consultas c WHERE c.criado_em < p_antes AND c.dados_apagados_em IS NULL
$$;
REVOKE ALL ON FUNCTION retencao_tenants_pendentes(timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION retencao_tenants_pendentes(timestamptz) TO motor_app;

-- A varredura diária procura por data só entre as linhas ainda não anonimizadas.
CREATE INDEX consultas_retencao ON consultas (criado_em) WHERE dados_apagados_em IS NULL;
