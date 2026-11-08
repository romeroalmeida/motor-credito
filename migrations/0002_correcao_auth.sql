-- Correções da conferência de auth de 07/11/2026 (docs/conferencias/2026-11-07-auth.md).
-- O projeto não tem produção: segredos no formato antigo são invalidados em vez de migrados.

-- ---------------------------------------------------------------- achado 11: TOTP com subchave e AAD
-- O segredo TOTP passou a ser cifrado com subchave própria e AAD = usuario_id; o formato antigo não decifra mais.
-- Quem tinha TOTP precisa de um novo (a semente recria o do admin demo).
DELETE FROM totp;

-- ---------------------------------------------------------------- achado 2: o segredo da chave de API assina
-- O Bearer leva só "ck_live_<id>"; o segredo é a chave do HMAC e o servidor precisa dele: fica cifrado (AES-GCM,
-- subchave própria, AAD = id da chave). O hash antigo não serve para isso: as chaves existentes são revogadas.
DROP FUNCTION auth_chave_api(text);
ALTER TABLE chaves_api ADD COLUMN segredo_cifrado bytea;
UPDATE chaves_api SET revogada_em = coalesce(revogada_em, now());
ALTER TABLE chaves_api DROP COLUMN segredo_hash;
ALTER TABLE chaves_api ADD CONSTRAINT chaves_api_segredo CHECK (segredo_cifrado IS NOT NULL OR revogada_em IS NOT NULL);

CREATE FUNCTION auth_chave_api(p_id text)
RETURNS TABLE (tenant_id text, segredo_cifrado bytea, revogada_em timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    SELECT c.tenant_id, c.segredo_cifrado, c.revogada_em FROM public.chaves_api c WHERE c.id = p_id
$$;
REVOKE ALL ON FUNCTION auth_chave_api(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_chave_api(text) TO motor_app;

-- ---------------------------------------------------------------- achado 6: trava persistente do TOTP
-- Falhas seguidas contam no banco (valem entre processos e reinícios); na 5ª o 2FA trava por 15 min.
ALTER TABLE totp ADD COLUMN falhas int NOT NULL DEFAULT 0, ADD COLUMN travado_ate timestamptz;

-- ---------------------------------------------------------------- achado 9: sessão expira por inatividade
-- Além do teto de 12 h (expira_em), a sessão cai depois de 30 min sem uso. auth_sessao passa a marcar o uso
-- (por isso VOLATILE): cada request autenticada empurra ultimo_uso_em.
ALTER TABLE sessoes ADD COLUMN ultimo_uso_em timestamptz NOT NULL DEFAULT now();

CREATE OR REPLACE FUNCTION auth_sessao(p_token_hash bytea)
RETURNS TABLE (sessao_id text, usuario_id text, tenant_id text, papel text, csrf_hash bytea,
               dois_fatores_em timestamptz, expira_em timestamptz)
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    UPDATE public.sessoes s SET ultimo_uso_em = now()
    FROM public.associacoes a
    WHERE a.usuario_id = s.usuario_id AND a.tenant_id = s.tenant_id
      AND s.token_hash = p_token_hash AND s.encerrada_em IS NULL AND s.expira_em > now()
      AND s.ultimo_uso_em > now() - interval '30 minutes'
    RETURNING s.id, s.usuario_id, s.tenant_id, a.papel, s.csrf_hash, s.dois_fatores_em, s.expira_em
$$;
