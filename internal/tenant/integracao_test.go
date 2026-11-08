package tenant

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
)

// Um container só para o pacote: os casos rodam como subtestes.
func TestIntegracao(t *testing.T) {
	b := bancoteste.Novo(t)
	ctx := context.Background()
	criarTenant(t, b.Dono, "ten_a", 1000)
	criarTenant(t, b.Dono, "ten_b", 1000)
	criarTenant(t, b.Dono, "ten_lim", 2)

	t.Run("middleware", func(t *testing.T) { testarMiddleware(t, NovoServico(b.App, chaveMestraTeste)) })
	t.Run("limite por minuto", func(t *testing.T) { testarLimite(t, NovoServico(b.App, chaveMestraTeste)) })
	t.Run("idempotencia", func(t *testing.T) { testarIdempotencia(t, NovoServico(b.App, chaveMestraTeste)) })
	t.Run("ips", func(t *testing.T) { testarIPs(t, NovoServico(b.App, chaveMestraTeste)) })
	t.Run("segredo cifrado amarrado à chave", func(t *testing.T) {
		testarSegredoCifrado(t, b.Dono, NovoServico(b.App, chaveMestraTeste))
	})
	// Duas barreiras: com o usuário da aplicação vale RLS + filtro; com o dono (sem RLS) só o filtro explícito.
	for nome, pool := range map[string]*pgxpool.Pool{"app (RLS)": b.App, "dono (sem RLS)": b.Dono} {
		t.Run("chaves e isolamento "+nome, func(t *testing.T) { testarChavesIsolamento(t, ctx, NovoServico(pool, chaveMestraTeste)) })
	}
}

func criarTenant(t *testing.T, dono *pgxpool.Pool, id string, limite int) {
	t.Helper()
	if _, err := dono.Exec(context.Background(), `INSERT INTO tenants (id, nome, limite_por_minuto) VALUES ($1, $1, $2)`,
		id, limite); err != nil {
		t.Fatal(err)
	}
}

var chaveMestraTeste = bytes.Repeat([]byte{9}, 32)

type req struct {
	auth, ts, assinatura, corpo, remote, idem string
}

func assinada(c ChaveCriada, ts time.Time, corpo string) req {
	return assinadaCom(c, ts, "", corpo)
}

func assinadaCom(c ChaveCriada, ts time.Time, idem, corpo string) req {
	tss := strconv.FormatInt(ts.Unix(), 10)
	return req{auth: "Bearer " + c.Chave, ts: tss, idem: idem, assinatura: Assinar(c.Segredo, tss, idem, []byte(corpo)), corpo: corpo}
}

func chamar(s *Servico, q req) *httptest.ResponseRecorder {
	eco := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ten, _ := TenantDoContexto(r.Context())
		corpo, _ := io.ReadAll(r.Body)
		_, _ = w.Write([]byte(ten.ID + "|" + string(corpo)))
	})
	r := httptest.NewRequest(http.MethodPost, "/v1/consultas", strings.NewReader(q.corpo))
	r.Header.Set("Authorization", q.auth)
	r.Header.Set("X-Timestamp", q.ts)
	r.Header.Set("X-Assinatura", q.assinatura)
	if q.idem != "" {
		r.Header.Set("Idempotency-Key", q.idem)
	}
	if q.remote != "" {
		r.RemoteAddr = q.remote
	}
	w := httptest.NewRecorder()
	s.MiddlewareAPI(eco).ServeHTTP(w, r)
	return w
}

func testarMiddleware(t *testing.T, s *Servico) {
	ctx := context.Background()
	agora := time.Now()
	s.agora = func() time.Time { return agora }
	chave, err := s.CriarChave(ctx, "ten_a")
	if err != nil {
		t.Fatal(err)
	}
	revogada, _ := s.CriarChave(ctx, "ten_a")
	if err := s.RevogarChave(ctx, "ten_a", revogada.ID, "teste"); err != nil {
		t.Fatal(err)
	}
	corpo := `{"documento":"52998224725"}`
	idFalso := strings.Repeat("0", 26)
	segredoFalso := strings.Repeat("a", 64)

	casos := []struct {
		nome   string
		q      req
		status int
	}{
		{"válida", assinada(chave, agora, corpo), 200},
		{"timestamp no limite passado (-5 min)", assinada(chave, agora.Add(-ToleranciaTimestamp), corpo), 200},
		{"timestamp no limite futuro (+5 min)", assinada(chave, agora.Add(ToleranciaTimestamp), corpo), 200},
		{"timestamp vencido (-5 min -1 s)", assinada(chave, agora.Add(-ToleranciaTimestamp-time.Second), corpo), 401},
		{"timestamp futuro (+5 min +1 s)", assinada(chave, agora.Add(ToleranciaTimestamp+time.Second), corpo), 401},
		{"assinatura errada", func() req {
			q := assinada(chave, agora, corpo)
			q.assinatura = strings.Repeat("0", 64)
			return q
		}(), 401},
		{"assinatura não hex", func() req { q := assinada(chave, agora, corpo); q.assinatura = "zz"; return q }(), 401},
		{"corpo alterado", func() req {
			q := assinada(chave, agora, corpo)
			q.corpo = `{"documento":"11144477735"}`
			return q
		}(), 401},
		{"timestamp alterado depois de assinar", func() req {
			q := assinada(chave, agora, corpo)
			q.ts = strconv.FormatInt(agora.Unix()+1, 10)
			return q
		}(), 401},
		{"chave revogada", assinada(revogada, agora, corpo), 401},
		{"chave inexistente", assinada(ChaveCriada{Chave: "ck_live_" + idFalso, Segredo: segredoFalso}, agora, corpo), 401},
		{"segredo errado com id real", assinada(ChaveCriada{Chave: chave.Chave, Segredo: segredoFalso}, agora, corpo), 401},
		{"sem Bearer", func() req { q := assinada(chave, agora, corpo); q.auth = chave.Chave; return q }(), 401},
		{"segredo no Bearer (formato antigo)", func() req {
			q := assinada(chave, agora, corpo)
			q.auth = "Bearer " + chave.Chave + "_" + chave.Segredo
			return q
		}(), 401},
		{"válida com Idempotency-Key", assinadaCom(chave, agora, "pedido-1", corpo), 200},
		{"Idempotency-Key trocada depois de assinar", func() req {
			q := assinadaCom(chave, agora, "pedido-1", corpo)
			q.idem = "pedido-2"
			return q
		}(), 401},
		{"Idempotency-Key acrescentada depois de assinar", func() req {
			q := assinada(chave, agora, corpo)
			q.idem = "pedido-1"
			return q
		}(), 401},
		{"assinada com o id no lugar do segredo", func() req {
			q := assinada(chave, agora, corpo)
			q.assinatura = Assinar(chave.Chave, q.ts, "", []byte(corpo))
			return q
		}(), 401},
		{"sem timestamp", func() req { q := assinada(chave, agora, corpo); q.ts = ""; return q }(), 401},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			w := chamar(s, c.q)
			if w.Code != c.status {
				t.Fatalf("status = %d; quer %d (%s)", w.Code, c.status, w.Body)
			}
			if c.status == 401 && strings.TrimSpace(w.Body.String()) != `{"erro":"nao_autorizado"}` {
				t.Fatalf("corpo = %s; quer só nao_autorizado", w.Body)
			}
			if c.status == 200 && w.Body.String() != "ten_a|"+corpo {
				t.Fatalf("tenant/corpo no handler = %q", w.Body)
			}
		})
	}

	t.Run("corpo grande", func(t *testing.T) {
		w := chamar(s, assinada(chave, agora, strings.Repeat("x", MaxCorpo+1)))
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("ultimo uso gravado", func(t *testing.T) {
		chaves, err := s.ListarChaves(ctx, "ten_a")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range chaves {
			if c.ID == chave.ID && c.UltimoUsoEm == nil {
				t.Fatal("ultimo_uso_em não gravado")
			}
		}
	})

	t.Run("ip permitido", func(t *testing.T) {
		if _, err := s.DefinirIPs(ctx, "ten_a", []string{"200.150.10.0/24"}, "teste"); err != nil {
			t.Fatal(err)
		}
		defer func() { _, _ = s.DefinirIPs(ctx, "ten_a", nil, "teste") }()
		for remote, quer := range map[string]int{"200.150.10.7:5555": 200, "200.150.11.7:5555": 401, "lixo": 401} {
			q := assinada(chave, agora, corpo)
			q.remote = remote
			if w := chamar(s, q); w.Code != quer {
				t.Errorf("remote %s: status = %d; quer %d", remote, w.Code, quer)
			}
		}
	})
}

func testarLimite(t *testing.T, s *Servico) {
	agora := time.Now()
	s.agora = func() time.Time { return agora }
	chave, err := s.CriarChave(context.Background(), "ten_lim")
	if err != nil {
		t.Fatal(err)
	}
	for i, quer := range []int{200, 200, 429} {
		w := chamar(s, assinada(chave, agora, "{}"))
		if w.Code != quer {
			t.Fatalf("requisição %d: status = %d; quer %d", i+1, w.Code, quer)
		}
		if quer == 429 && w.Header().Get("Retry-After") == "" {
			t.Fatal("429 sem Retry-After")
		}
	}
	agora = agora.Add(time.Minute)
	if w := chamar(s, assinada(chave, agora, "{}")); w.Code != 200 {
		t.Fatalf("janela nova: status = %d", w.Code)
	}
}

func testarIdempotencia(t *testing.T, s *Servico) {
	ctx := context.Background()
	h1, h2 := []byte("hash-corpo-1"), []byte("hash-corpo-2")
	if _, achou, err := s.Idempotencia(ctx, "ten_a", "k1", h1); err != nil || achou {
		t.Fatalf("antes de guardar: achou=%v err=%v", achou, err)
	}
	if err := s.GuardarIdempotencia(ctx, "ten_a", "k1", h1, []byte(`{"id":"cons_1"}`)); err != nil {
		t.Fatal(err)
	}
	resp, achou, err := s.Idempotencia(ctx, "ten_a", "k1", h1)
	if err != nil || !achou || string(resp) != `{"id":"cons_1"}` {
		t.Fatalf("mesmo corpo: resp=%s achou=%v err=%v", resp, achou, err)
	}
	if _, _, err := s.Idempotencia(ctx, "ten_a", "k1", h2); !errors.Is(err, ErrIdempotenciaConflito) {
		t.Fatalf("corpo diferente: err = %v; quer conflito", err)
	}
	if err := s.GuardarIdempotencia(ctx, "ten_a", "k1", h2, []byte("x")); !errors.Is(err, ErrIdempotenciaConflito) {
		t.Fatalf("guardar corpo diferente: err = %v; quer conflito", err)
	}
	if err := s.GuardarIdempotencia(ctx, "ten_a", "k1", h1, []byte("outra")); err != nil {
		t.Fatalf("guardar de novo o mesmo corpo deve valer a primeira: %v", err)
	}
	if _, achou, err := s.Idempotencia(ctx, "ten_b", "k1", h1); err != nil || achou {
		t.Fatalf("outro tenant não pode ver a chave: achou=%v err=%v", achou, err)
	}
	for _, k := range []string{"", strings.Repeat("k", MaxChaveIdempotencia+1)} {
		if _, _, err := s.Idempotencia(ctx, "ten_a", k, h1); !errors.Is(err, ErrIdempotenciaChaveInvalida) {
			t.Errorf("chave de %d bytes: err = %v", len(k), err)
		}
	}
}

func testarIPs(t *testing.T, s *Servico) {
	ctx := context.Background()
	got, err := s.DefinirIPs(ctx, "ten_b", []string{"10.1.2.3", "10.0.0.9/8", "2001:db8::1"}, "teste")
	if err != nil {
		t.Fatal(err)
	}
	ten, err := s.Buscar(ctx, "ten_b")
	if err != nil {
		t.Fatal(err)
	}
	quer := "10.1.2.3/32 10.0.0.0/8 2001:db8::1/128"
	if txt := juntar(got); txt != quer || juntar(ten.IPsPermitidos) != quer {
		t.Fatalf("IPs = %q / banco %q; quer %q", txt, juntar(ten.IPsPermitidos), quer)
	}
	if _, err := s.DefinirIPs(ctx, "ten_b", []string{"999.1.1.1"}, "teste"); !errors.Is(err, ErrIPInvalido) {
		t.Fatalf("IP inválido: err = %v", err)
	}
	if _, err := s.DefinirIPs(ctx, "ten_b", make([]string, MaxIPsPermitidos+1), "teste"); !errors.Is(err, ErrIPInvalido) {
		t.Fatalf("lista grande: err = %v", err)
	}
}

func juntar[T interface{ String() string }](xs []T) string {
	partes := make([]string, len(xs))
	for i, x := range xs {
		partes[i] = x.String()
	}
	return strings.Join(partes, " ")
}

func testarChavesIsolamento(t *testing.T, ctx context.Context, s *Servico) {
	criada, err := s.CriarChaveComNome(ctx, "ten_a", "produção", "teste")
	if err != nil {
		t.Fatal(err)
	}
	if criada.Chave != "ck_live_"+criada.ID || len(criada.Segredo) != 64 {
		t.Fatalf("chave criada = %+v", criada)
	}

	// Tenant B tentando mexer na chave do A: sempre 404 (ErrNaoEncontrado), e a chave segue intacta.
	if err := s.RevogarChave(ctx, "ten_b", criada.ID, "teste"); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("revogar chave alheia: err = %v; quer ErrNaoEncontrado", err)
	}
	if _, err := s.GirarChave(ctx, "ten_b", criada.ID, "teste"); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("girar chave alheia: err = %v; quer ErrNaoEncontrado", err)
	}
	if lista, _ := s.ListarChaves(ctx, "ten_b"); contem(lista, criada.ID) {
		t.Fatal("tenant B lista chave do A")
	}
	if lista, _ := s.ListarChaves(ctx, "ten_a"); !contem(lista, criada.ID) || revogadaEm(lista, criada.ID) {
		t.Fatal("chave do A sumiu ou foi revogada pelo B")
	}

	nova, err := s.GirarChave(ctx, "ten_a", criada.ID, "teste")
	if err != nil {
		t.Fatal(err)
	}
	lista, _ := s.ListarChaves(ctx, "ten_a")
	if !revogadaEm(lista, criada.ID) || revogadaEm(lista, nova.ID) || nome(lista, nova.ID) != "produção" {
		t.Fatalf("girar: antiga deveria estar revogada e a nova ativa com o mesmo nome: %+v", lista)
	}
	if _, err := s.GirarChave(ctx, "ten_a", criada.ID, "teste"); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("girar chave já revogada: err = %v", err)
	}
	if err := s.RevogarChave(ctx, "ten_a", nova.ID, "teste"); err != nil {
		t.Fatal(err)
	}
	if err := s.RevogarChave(ctx, "ten_a", nova.ID, "teste"); err != nil {
		t.Fatalf("revogar de novo deve ser idempotente: %v", err)
	}
}

func contem(cs []Chave, id string) bool { return achar(cs, id) != nil }
func revogadaEm(cs []Chave, id string) bool {
	c := achar(cs, id)
	return c != nil && c.RevogadaEm != nil
}
func nome(cs []Chave, id string) string {
	if c := achar(cs, id); c != nil {
		return c.Nome
	}
	return ""
}
func achar(cs []Chave, id string) *Chave {
	for i := range cs {
		if cs[i].ID == id {
			return &cs[i]
		}
	}
	return nil
}

// O banco guarda o segredo só cifrado, e a cifra é amarrada ao id: copiar o segredo_cifrado de uma chave para
// outra não autentica (AAD = id da chave). Chave mestra diferente também não.
func testarSegredoCifrado(t *testing.T, dono *pgxpool.Pool, s *Servico) {
	ctx := context.Background()
	agora := time.Now()
	s.agora = func() time.Time { return agora }
	a, _ := s.CriarChave(ctx, "ten_a")
	b, _ := s.CriarChave(ctx, "ten_a")
	var cifrado []byte
	if err := dono.QueryRow(ctx, `SELECT segredo_cifrado FROM chaves_api WHERE id = $1`, a.ID).Scan(&cifrado); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cifrado, []byte(a.Segredo)) {
		t.Fatal("segredo em claro no banco")
	}
	if w := chamar(s, assinada(a, agora, "{}")); w.Code != 200 {
		t.Fatalf("chave A antes da cópia: %d", w.Code)
	}
	if _, err := dono.Exec(ctx, `UPDATE chaves_api SET segredo_cifrado = $2 WHERE id = $1`, b.ID, cifrado); err != nil {
		t.Fatal(err)
	}
	if w := chamar(s, assinada(ChaveCriada{Chave: b.Chave, Segredo: a.Segredo}, agora, "{}")); w.Code != 401 {
		t.Fatalf("segredo de A copiado para B autenticou: %d", w.Code)
	}
	outra := NovoServico(s.pool, bytes.Repeat([]byte{1}, 32))
	outra.agora = s.agora
	if w := chamar(outra, assinada(a, agora, "{}")); w.Code != 401 {
		t.Fatalf("chave mestra errada autenticou: %d", w.Code)
	}
}
