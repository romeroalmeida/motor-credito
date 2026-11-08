package brasilapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
)

const cnpjBB = "00000000000191"

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/cnpj_00000000000191.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func servidor(t *testing.T, h http.HandlerFunc) bureau.Bureau {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return Novo(s.URL)
}

func responde(status int, corpo string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(corpo))
	}
}

func TestRespostaReal(t *testing.T) {
	var caminho, ua string
	b := servidor(t, func(w http.ResponseWriter, r *http.Request) {
		caminho, ua = r.URL.Path, r.UserAgent()
		_, _ = w.Write([]byte(fixture(t)))
	})
	r, err := b.Consultar(context.Background(), "00.000.000/0001-91")
	if err != nil {
		t.Fatal(err)
	}
	quer := bureau.Resultado{Nome: "BANCO DO BRASIL SA", Situacao: bureau.Regular, Abertura: "1966-08-01"}
	if r != quer {
		t.Fatalf("resultado %+v, quer %+v", r, quer)
	}
	if caminho != "/"+cnpjBB || ua != userAgent {
		t.Fatalf("caminho %q, user-agent %q", caminho, ua)
	}
}

func TestSituacoes(t *testing.T) {
	casos := []struct {
		descricao string
		quer      bureau.Situacao
	}{
		{"ATIVA", bureau.Regular},
		{"BAIXADA", bureau.Inativo},
		{"INAPTA", bureau.Inativo},
		{"SUSPENSA", bureau.Inativo},
		{"NULA", bureau.Inativo},
		{" baixada ", bureau.Inativo},
	}
	for _, c := range casos {
		corpo := strings.Replace(fixture(t), `"descricao_situacao_cadastral": "ATIVA"`, `"descricao_situacao_cadastral": "`+c.descricao+`"`, 1)
		if !strings.Contains(corpo, c.descricao) {
			t.Fatal("fixture mudou de formato")
		}
		r, err := servidor(t, responde(http.StatusOK, corpo)).Consultar(context.Background(), cnpjBB)
		if err != nil || r.Situacao != c.quer {
			t.Errorf("%q: situação %q, erro %v; quer %q", c.descricao, r.Situacao, err, c.quer)
		}
	}
}

func TestNaoEncontrado(t *testing.T) {
	r, err := servidor(t, responde(http.StatusNotFound, `{"message":"CNPJ não encontrado"}`)).Consultar(context.Background(), cnpjBB)
	if err != nil || r != (bureau.Resultado{Situacao: bureau.Inexistente}) {
		t.Fatalf("resultado %+v, erro %v", r, err)
	}
}

func TestFailClosed(t *testing.T) {
	gigante := `{"razao_social":"` + strings.Repeat("A", 2<<20) + `","descricao_situacao_cadastral":"ATIVA"}`
	casos := []struct {
		nome string
		h    http.HandlerFunc
	}{
		{"429", responde(http.StatusTooManyRequests, `{}`)},
		{"500", responde(http.StatusInternalServerError, fixture(t))},
		{"503", responde(http.StatusServiceUnavailable, ``)},
		{"400", responde(http.StatusBadRequest, `{"message":"CNPJ inválido"}`)},
		{"json inválido", responde(http.StatusOK, `{"razao_social":`)},
		{"situação desconhecida", responde(http.StatusOK, `{"razao_social":"X","descricao_situacao_cadastral":"NOVA"}`)},
		{"situação ausente", responde(http.StatusOK, `{"razao_social":"X"}`)},
		{"data inválida", responde(http.StatusOK, `{"razao_social":"X","descricao_situacao_cadastral":"ATIVA","data_inicio_atividade":"01/08/1966"}`)},
		{"corpo gigante", responde(http.StatusOK, gigante)},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r, err := servidor(t, c.h).Consultar(context.Background(), cnpjBB)
			if !errors.Is(err, bureau.ErrIndisponivel) || r != (bureau.Resultado{}) {
				t.Fatalf("resultado %+v, erro %v; quer ErrIndisponivel", r, err)
			}
		})
	}
}

func TestTimeoutDoCtx(t *testing.T) {
	b := servidor(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	inicio := time.Now()
	_, err := b.Consultar(ctx, cnpjBB)
	if !errors.Is(err, bureau.ErrIndisponivel) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro = %v, quer ErrIndisponivel + DeadlineExceeded", err)
	}
	if d := time.Since(inicio); d > time.Second {
		t.Fatalf("demorou %v, não respeitou o ctx", d)
	}
}

func TestRecusaSemChamar(t *testing.T) {
	chamou := false
	b := servidor(t, func(http.ResponseWriter, *http.Request) { chamou = true })
	for _, doc := range []string{"12345678900", "00000000191"} { // dígito errado; CPF válido
		if _, err := b.Consultar(context.Background(), doc); err == nil || errors.Is(err, bureau.ErrIndisponivel) {
			t.Errorf("%s: erro %v, quer erro de documento", doc, err)
		}
	}
	if chamou {
		t.Fatal("chamou a API com documento que não é CNPJ válido")
	}
}

// Ao vivo: BRASILAPI_AO_VIVO=on go test ./internal/bureau/brasilapi -run AoVivo
func TestAoVivo(t *testing.T) {
	if os.Getenv("BRASILAPI_AO_VIVO") != "on" {
		t.Skip("defina BRASILAPI_AO_VIVO=on para chamar a BrasilAPI de verdade")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := Novo("").Consultar(ctx, cnpjBB)
	if err != nil || r.Nome == "" || r.Situacao != bureau.Regular || r.Abertura != "1966-08-01" {
		t.Fatalf("resultado %+v, erro %v", r, err)
	}
}
