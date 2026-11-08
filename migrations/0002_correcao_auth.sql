-- Correções da conferência de auth de 07/11/2026 (docs/conferencias/2026-11-07-auth.md).
-- O projeto não tem produção: segredos no formato antigo são invalidados em vez de migrados.

-- ---------------------------------------------------------------- achado 11: TOTP com subchave e AAD
-- O segredo TOTP passou a ser cifrado com subchave própria e AAD = usuario_id; o formato antigo não decifra mais.
-- Quem tinha TOTP precisa de um novo (a semente recria o do admin demo).
DELETE FROM totp;
