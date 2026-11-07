-- Schema inicial do motor-credito (desenho §4–§8).
--
-- Papéis:
--   * quem roda esta migration é o DONO das tabelas (não está sujeito à RLS, pois ela não é FORCE);
--   * a aplicação conecta com um usuário de login membro de `motor_app` (sem BYPASSRLS, não dono).
-- Isolamento: toda tabela com tenant_id tem RLS por current_setting('app.tenant_id', true).
--   Sem SET LOCAL app.tenant_id a comparação dá NULL e nenhuma linha passa (fail-closed).
-- Dinheiro em centavos (bigint); taxa em pontos-base (int). CPF/CNPJ nunca em claro: doc_hash (HMAC-SHA256)
-- e doc_cifrado (AES-256-GCM), ambos gerados pela aplicação.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'motor_app') THEN
        CREATE ROLE motor_app NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
END
$$;

-- ---------------------------------------------------------------- tenant (dono: A)

CREATE TABLE tenants (
    id                text PRIMARY KEY,                       -- "ten_<ulid>"
    nome              text        NOT NULL,
    limite_por_minuto int         NOT NULL DEFAULT 60 CHECK (limite_por_minuto > 0),
    ips_permitidos    cidr[]      NOT NULL DEFAULT '{}',      -- vazio = qualquer IP
    retencao_dias     int         NOT NULL DEFAULT 1825 CHECK (retencao_dias > 0), -- LGPD: 5 anos
    criado_em         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE chaves_api (
    id           text PRIMARY KEY,                            -- o <id> de "ck_live_<id>_<segredo>"
    tenant_id    text        NOT NULL REFERENCES tenants (id),
    segredo_hash bytea       NOT NULL CHECK (length(segredo_hash) = 32), -- SHA-256 do segredo
    nome         text        NOT NULL DEFAULT '',
    criada_em    timestamptz NOT NULL DEFAULT now(),
    revogada_em  timestamptz,
    ultimo_uso_em timestamptz
);
CREATE INDEX chaves_api_tenant ON chaves_api (tenant_id);

CREATE TABLE idempotencia (
    tenant_id  text        NOT NULL REFERENCES tenants (id),
    chave      text        NOT NULL CHECK (length(chave) BETWEEN 1 AND 255),
    hash_corpo bytea       NOT NULL,
    resposta   bytea       NOT NULL,
    criado_em  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, chave)
);

-- ---------------------------------------------------------------- painel (dono: A)

-- Usuário é global (o e-mail entra antes de se saber o tenant); o vínculo com o tenant fica em associacoes.
CREATE TABLE usuarios (
    id         text PRIMARY KEY,                              -- "usr_<ulid>"
    email      text        NOT NULL,
    nome       text        NOT NULL DEFAULT '',
    senha_hash text        NOT NULL,                          -- argon2id em formato PHC
    criado_em  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX usuarios_email ON usuarios (lower(email));

CREATE TABLE totp (
    usuario_id      text PRIMARY KEY REFERENCES usuarios (id) ON DELETE CASCADE,
    segredo_cifrado bytea       NOT NULL,                     -- AES-256-GCM
    confirmado_em   timestamptz,                              -- NULL = cadastro do 2FA não concluído
    ultimo_passo    bigint      NOT NULL DEFAULT 0            -- anti-replay: passo TOTP já usado
);

CREATE TABLE associacoes (
    usuario_id text        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    tenant_id  text        NOT NULL REFERENCES tenants (id),
    papel      text        NOT NULL CHECK (papel IN ('admin', 'analista', 'auditor')),
    criado_em  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (usuario_id, tenant_id)
);
CREATE INDEX associacoes_tenant ON associacoes (tenant_id);

CREATE TABLE sessoes (
    id              text PRIMARY KEY,                         -- "ses_<ulid>"
    token_hash      bytea       NOT NULL UNIQUE,              -- SHA-256 do valor do cookie
    csrf_hash       bytea       NOT NULL,                     -- SHA-256 do token CSRF
    usuario_id      text        NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    tenant_id       text        NOT NULL REFERENCES tenants (id),
    dois_fatores_em timestamptz,                              -- NULL = senha ok, falta o TOTP
    criada_em       timestamptz NOT NULL DEFAULT now(),
    expira_em       timestamptz NOT NULL,
    encerrada_em    timestamptz,
    ip              inet,
    user_agent      text        NOT NULL DEFAULT ''
);
CREATE INDEX sessoes_usuario ON sessoes (usuario_id);

CREATE TABLE convites (
    id         text PRIMARY KEY,                              -- "cvt_<ulid>"
    tenant_id  text        NOT NULL REFERENCES tenants (id),
    email      text        NOT NULL,
    papel      text        NOT NULL CHECK (papel IN ('admin', 'analista', 'auditor')),
    token_hash bytea       NOT NULL UNIQUE,
    criado_por text        NOT NULL REFERENCES usuarios (id),
    criado_em  timestamptz NOT NULL DEFAULT now(),
    expira_em  timestamptz NOT NULL,
    usado_em   timestamptz
);
CREATE INDEX convites_tenant ON convites (tenant_id);

-- ---------------------------------------------------------------- política (dono: B/E)

CREATE TABLE politicas (
    id                      text PRIMARY KEY,                 -- "pol_<ulid>"
    tenant_id               text        NOT NULL REFERENCES tenants (id),
    nome                    text        NOT NULL,
    rascunho                jsonb       NOT NULL,             -- politica.Politica em edição
    rascunho_atualizado_em  timestamptz NOT NULL DEFAULT now(),
    rascunho_atualizado_por text REFERENCES usuarios (id),
    versao_publicada        int,                              -- NULL = nunca publicada
    criado_em               timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, nome)
);

-- Imutável: motor_app só faz SELECT/INSERT. Reverter = inserir cópia como versão nova.
CREATE TABLE politica_versoes (
    politica_id   text        NOT NULL REFERENCES politicas (id),
    tenant_id     text        NOT NULL REFERENCES tenants (id),
    versao        int         NOT NULL CHECK (versao > 0),
    conteudo      jsonb       NOT NULL,
    publicada_por text        NOT NULL REFERENCES usuarios (id),
    publicada_em  timestamptz NOT NULL DEFAULT now(),
    revertida_de  int,                                        -- versão de origem quando é reversão
    comentario    text        NOT NULL DEFAULT '',
    PRIMARY KEY (politica_id, versao)
);
CREATE INDEX politica_versoes_tenant ON politica_versoes (tenant_id);

-- ---------------------------------------------------------------- consulta (dono: E)

CREATE TABLE consultas (
    id               text PRIMARY KEY,                        -- "cons_<ulid>"
    tenant_id        text        NOT NULL REFERENCES tenants (id),
    doc_hash         bytea,                                   -- HMAC-SHA256; NULL depois da retenção
    doc_cifrado      bytea,                                   -- AES-256-GCM; NULL depois da retenção
    doc_tipo         text CHECK (doc_tipo IN ('cpf', 'cnpj')),
    finalidade       text        NOT NULL,                    -- "concessao_credito"
    base_legal       text        NOT NULL,                    -- "protecao_credito_art7_x"
    entrada          jsonb       NOT NULL,                    -- pedido e dados opcionais SEM PII em claro
    status           text        NOT NULL CHECK (status IN ('aprovado', 'negado', 'contraproposta', 'documento_invalido', 'em_analise')),
    score            int,
    faixa_risco      text,
    decisao          jsonb       NOT NULL,                    -- resposta completa + trilha de regras
    antifraude_risco int CHECK (antifraude_risco BETWEEN 0 AND 100),
    politica_id      text REFERENCES politicas (id),
    politica_versao  int,
    idempotency_key  text,
    criado_em        timestamptz NOT NULL DEFAULT now(),
    dados_apagados_em timestamptz,                            -- retenção LGPD executada
    FOREIGN KEY (politica_id, politica_versao) REFERENCES politica_versoes (politica_id, versao)
);
CREATE INDEX consultas_tenant_criado ON consultas (tenant_id, criado_em DESC);
CREATE INDEX consultas_tenant_status ON consultas (tenant_id, status, criado_em DESC);
CREATE INDEX consultas_tenant_doc ON consultas (tenant_id, doc_hash);

-- ---------------------------------------------------------------- auditoria (dono: E)

-- Só inserção: motor_app não tem UPDATE/DELETE. hash = SHA-256(hash_anterior || dados canônicos).
-- O primeiro registro de cada tenant usa hash_anterior = 32 bytes zero. UNIQUE impede bifurcar a cadeia.
CREATE TABLE auditoria (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id     text        NOT NULL REFERENCES tenants (id),
    seq           bigint      NOT NULL CHECK (seq > 0),
    evento        text        NOT NULL,                       -- "consulta.decidida", "politica.publicada", ...
    ator          text        NOT NULL,                       -- "api:<chave_id>" | "usuario:<id>"
    consulta_id   text REFERENCES consultas (id),
    dados         jsonb       NOT NULL,
    hash_anterior bytea       NOT NULL CHECK (length(hash_anterior) = 32),
    hash          bytea       NOT NULL CHECK (length(hash) = 32),
    criado_em     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, seq),
    UNIQUE (tenant_id, hash_anterior)
);

-- ---------------------------------------------------------------- antifraude (dono: C/E)

-- Contadores de velocidade ENTRE tenants: sem tenant_id, então sem RLS e sem revelar quem consultou.
-- shortcut: velocidade sai do Postgres; trocar por Redis quando o volume pedir.
CREATE TABLE antifraude_eventos (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    doc_hash         bytea       NOT NULL,
    dispositivo_hash bytea,                                   -- HMAC do device_id; NULL se não enviado
    criado_em        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX antifraude_eventos_doc ON antifraude_eventos (doc_hash, criado_em);
CREATE INDEX antifraude_eventos_dispositivo ON antifraude_eventos (dispositivo_hash, criado_em) WHERE dispositivo_hash IS NOT NULL;

-- ---------------------------------------------------------------- RLS

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
CREATE POLICY isolamento ON tenants
    USING (id = current_setting('app.tenant_id', true))
    WITH CHECK (id = current_setting('app.tenant_id', true));

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['chaves_api', 'idempotencia', 'associacoes', 'sessoes', 'convites',
                             'politicas', 'politica_versoes', 'consultas', 'auditoria']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY isolamento ON %I
            USING (tenant_id = current_setting('app.tenant_id', true))
            WITH CHECK (tenant_id = current_setting('app.tenant_id', true))$p$, t);
    END LOOP;
END
$$;

-- ---------------------------------------------------------------- permissões do motor_app

GRANT USAGE ON SCHEMA public TO motor_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, chaves_api, idempotencia, usuarios, totp, associacoes,
    sessoes, convites, politicas, consultas TO motor_app;
GRANT SELECT, INSERT ON politica_versoes, auditoria TO motor_app;
GRANT SELECT, INSERT, DELETE ON antifraude_eventos TO motor_app;

-- ---------------------------------------------------------------- busca antes de saber o tenant
-- A chave de API, o cookie de sessão e o link de convite chegam antes do tenant. Estas funções rodam como o
-- dono (SECURITY DEFINER), buscam só pelo identificador secreto e devolvem só o necessário para autenticar.

CREATE FUNCTION auth_chave_api(p_id text)
RETURNS TABLE (tenant_id text, segredo_hash bytea, revogada_em timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    SELECT c.tenant_id, c.segredo_hash, c.revogada_em FROM public.chaves_api c WHERE c.id = p_id
$$;

CREATE FUNCTION auth_sessao(p_token_hash bytea)
RETURNS TABLE (sessao_id text, usuario_id text, tenant_id text, papel text, csrf_hash bytea,
               dois_fatores_em timestamptz, expira_em timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    SELECT s.id, s.usuario_id, s.tenant_id, a.papel, s.csrf_hash, s.dois_fatores_em, s.expira_em
    FROM public.sessoes s
    JOIN public.associacoes a ON a.usuario_id = s.usuario_id AND a.tenant_id = s.tenant_id
    WHERE s.token_hash = p_token_hash AND s.encerrada_em IS NULL AND s.expira_em > now()
$$;

CREATE FUNCTION auth_associacoes(p_usuario_id text)
RETURNS TABLE (tenant_id text, papel text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    SELECT a.tenant_id, a.papel FROM public.associacoes a WHERE a.usuario_id = p_usuario_id ORDER BY a.criado_em
$$;

CREATE FUNCTION auth_convite(p_token_hash bytea)
RETURNS TABLE (convite_id text, tenant_id text, email text, papel text, expira_em timestamptz, usado_em timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    SELECT c.id, c.tenant_id, c.email, c.papel, c.expira_em, c.usado_em FROM public.convites c WHERE c.token_hash = p_token_hash
$$;

REVOKE ALL ON FUNCTION auth_chave_api(text), auth_sessao(bytea), auth_associacoes(text), auth_convite(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_chave_api(text), auth_sessao(bytea), auth_associacoes(text), auth_convite(bytea) TO motor_app;
