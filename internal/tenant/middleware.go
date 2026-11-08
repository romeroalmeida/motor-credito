package tenant

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
)

const (
	// ToleranciaTimestamp é a diferença máxima entre X-Timestamp e o relógio do servidor.
	ToleranciaTimestamp = 5 * time.Minute
	// MaxCorpo é o tamanho máximo do corpo assinado (contrato: 64 KB).
	MaxCorpo = 64 << 10
)

type chaveContexto int

const (
	ctxTenant chaveContexto = iota
	ctxChaveID
)

// TenantDoContexto devolve o tenant que o MiddlewareAPI autenticou.
func TenantDoContexto(ctx context.Context) (Tenant, bool) {
	t, ok := ctx.Value(ctxTenant).(Tenant)
	return t, ok
}

// ChaveDoContexto devolve o id da chave de API usada (para o ator da auditoria: "api:<id>").
func ChaveDoContexto(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxChaveID).(string)
	return id, ok
}

// Assinar calcula o X-Assinatura: hex(HMAC-SHA256(segredo, timestamp + "." + idempotencyKey + "." + corpo)).
// segredo é o valor hex que o cliente recebeu junto com o id "ck_live_<id>"; sem Idempotency-Key, idempotencyKey é "".
// Assinar a Idempotency-Key impede que quem capturou uma requisição a repita trocando a chave.
func Assinar(segredo, timestamp, idempotencyKey string, corpo []byte) string {
	m := hmac.New(sha256.New, []byte(segredo))
	m.Write([]byte(timestamp))
	m.Write([]byte("."))
	m.Write([]byte(idempotencyKey))
	m.Write([]byte("."))
	m.Write(corpo)
	return hex.EncodeToString(m.Sum(nil))
}

// AssinarComChave assina com o segredo da chave chaveID do tenant (simulador do painel). O segredo é decifrado
// aqui e não sai do pacote: volta só a assinatura. Chave de outro tenant, revogada ou inexistente → ErrNaoEncontrado.
func (s *Servico) AssinarComChave(ctx context.Context, tenantID, chaveID, timestamp, idempotencyKey string, corpo []byte) (string, error) {
	var cifrado []byte
	err := EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT segredo_cifrado FROM chaves_api
			WHERE id = $1 AND tenant_id = $2 AND revogada_em IS NULL`, chaveID, tenantID).Scan(&cifrado)
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && cifrado == nil) {
		return "", ErrNaoEncontrado
	}
	if err != nil {
		return "", fmt.Errorf("buscar chave para assinar: %w", err)
	}
	segredo, err := plataforma.DecifrarAAD(s.chaveSegredo, cifrado, []byte(chaveID))
	if err != nil {
		return "", fmt.Errorf("decifrar segredo da chave %s: %w", chaveID, err)
	}
	return Assinar(string(segredo), timestamp, idempotencyKey, corpo), nil
}

// MiddlewareAPI autentica a API pública: Bearer + X-Timestamp (±5 min) + X-Assinatura + IP permitido + limite
// por minuto. Qualquer falha de credencial vira 401 {"erro":"nao_autorizado"}, sem dizer qual parte falhou.
func (s *Servico) MiddlewareAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxCorpo))
		if err != nil {
			var grande *http.MaxBytesError
			if errors.As(err, &grande) {
				escreverErro(w, http.StatusRequestEntityTooLarge, "corpo_grande")
				return
			}
			escreverErro(w, http.StatusBadRequest, "corpo_invalido")
			return
		}
		t, chaveID, err := s.autenticar(r, corpo)
		if err != nil {
			if !errors.Is(err, errCredencial) {
				slog.ErrorContext(r.Context(), "autenticar chave de API", "erro", err)
			}
			escreverErro(w, http.StatusUnauthorized, "nao_autorizado")
			return
		}
		if ok, espera := s.limitador.Permitir(t.ID, t.LimitePorMinuto, time.Minute, s.agora()); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(espera.Seconds())+1))
			escreverErro(w, http.StatusTooManyRequests, "limite_excedido")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(corpo))
		ctx := context.WithValue(context.WithValue(r.Context(), ctxTenant, t), ctxChaveID, chaveID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// errCredencial marca recusa esperada (não é falha do servidor, não vai para o log de erro).
var errCredencial = errors.New("credencial recusada")

func (s *Servico) autenticar(r *http.Request, corpo []byte) (Tenant, string, error) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return Tenant{}, "", errCredencial
	}
	chaveID, ok := idDaChave(token)
	if !ok || !s.timestampValido(r.Header.Get("X-Timestamp")) {
		return Tenant{}, "", errCredencial
	}
	recebida, err := hex.DecodeString(r.Header.Get("X-Assinatura"))
	if err != nil {
		return Tenant{}, "", errCredencial
	}

	var tenantID string
	var cifrado []byte
	var revogadaEm *time.Time
	err = s.pool.QueryRow(r.Context(), `SELECT tenant_id, segredo_cifrado, revogada_em FROM auth_chave_api($1)`, chaveID).
		Scan(&tenantID, &cifrado, &revogadaEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, "", errCredencial
	}
	if err != nil {
		return Tenant{}, "", err
	}
	if revogadaEm != nil || cifrado == nil {
		return Tenant{}, "", errCredencial
	}
	segredo, err := plataforma.DecifrarAAD(s.chaveSegredo, cifrado, []byte(chaveID))
	if err != nil {
		return Tenant{}, "", fmt.Errorf("decifrar segredo da chave %s: %w", chaveID, err)
	}
	esperada, _ := hex.DecodeString(Assinar(string(segredo), r.Header.Get("X-Timestamp"), r.Header.Get("Idempotency-Key"), corpo))
	if !hmac.Equal(recebida, esperada) {
		return Tenant{}, "", errCredencial
	}

	t, err := s.Buscar(r.Context(), tenantID)
	if err != nil {
		return Tenant{}, "", err
	}
	if !ipPermitido(r.RemoteAddr, t.IPsPermitidos) {
		return Tenant{}, "", errCredencial
	}
	s.marcarUso(r.Context(), tenantID, chaveID)
	return t, chaveID, nil
}

// marcarUso grava ultimo_uso_em; falha aqui não derruba a requisição já autenticada.
func (s *Servico) marcarUso(ctx context.Context, tenantID, chaveID string) {
	err := EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE chaves_api SET ultimo_uso_em = now() WHERE id = $1 AND tenant_id = $2`, chaveID, tenantID)
		return err
	})
	if err != nil {
		slog.WarnContext(ctx, "marcar uso da chave", "erro", err)
	}
}

// idDaChave lê "ck_live_<id>" (id ULID de 26 caracteres). O segredo nunca viaja: só assina.
func idDaChave(token string) (string, bool) {
	id, ok := strings.CutPrefix(token, prefixoChave)
	return id, ok && len(id) == 26 && !strings.Contains(id, "_")
}

func (s *Servico) timestampValido(v string) bool {
	seg, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return false
	}
	// Em segundos inteiros, como o cabeçalho: a fração do relógio do servidor não encurta a tolerância.
	diferenca := s.agora().Unix() - seg
	limite := int64(ToleranciaTimestamp / time.Second)
	return diferenca <= limite && diferenca >= -limite
}

// ipPermitido: lista vazia libera qualquer IP; endereço ilegível é recusado (fail-closed).
// Usa r.RemoteAddr, que httpapi.IPReal já reescreveu quando a conexão veio de proxy confiável.
func ipPermitido(remoteAddr string, permitidos []netip.Prefix) bool {
	if len(permitidos) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, p := range permitidos {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func escreverErro(w http.ResponseWriter, status int, codigo string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"erro": codigo})
}
