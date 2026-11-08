package serpro

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
)

// CPFs e nascimentos do ambiente de testes do Serpro (dados fictícios).
const (
	cpfRegular   = "40442820135"
	nascRegular  = "1970-11-14"
	cpfFalecido  = "05137518743"
	nascFalecido = "1965-07-05"
)

var hoje = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func fixture(t *testing.T, nome string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + nome)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func servidor(t *testing.T, h http.HandlerFunc) cliente {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c := Novo(s.URL+"/", "chave-teste", true).(cliente)
	c.agora = func() time.Time { return hoje }
	return c
}

func responde(status int, corpo string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(corpo))
	}
}

func consultar(t *testing.T, c cliente, cpf, nasc string) (bureau.Resultado, error) {
	t.Helper()
	return c.ConsultarComNascimento(context.Background(), cpf, nasc)
}

func TestRespostaReal(t *testing.T) {
	var caminho, auth string
	c := servidor(t, func(w http.ResponseWriter, r *http.Request) {
		caminho, auth = r.URL.Path, r.Header.Get("Authorization")
		_, _ = w.Write([]byte(fixture(t, "cpf_40442820135_regular.json")))
	})
	r, err := consultar(t, c, "404.428.201-35", nascRegular)
	if err != nil {
		t.Fatal(err)
	}
	quer := bureau.Resultado{Nome: "RONE PEREIRA", Nascimento: nascRegular, Situacao: bureau.Regular,
		Fontes: bureau.Fontes{Cadastro: bureau.FonteSerproTrial}}
	if !reflect.DeepEqual(r, quer) {
		t.Fatalf("resultado %+v, quer %+v", r, quer)
	}
	if caminho != "/cpf/40442820135/14111970" || auth != "Bearer chave-teste" {
		t.Fatalf("caminho %q, authorization %q", caminho, auth)
	}
	if Novo("x", "y", false).(cliente).fonte != bureau.FonteSerpro {
		t.Fatal("produção não se identifica como serpro")
	}
}

func TestFalecido(t *testing.T) {
	r, err := consultar(t, servidor(t, responde(200, fixture(t, "cpf_05137518743_falecido.json"))), cpfFalecido, nascFalecido)
	if err != nil || r.Situacao != bureau.Falecido {
		t.Fatalf("resultado %+v, erro %v", r, err)
	}
}

func TestSituacoes(t *testing.T) {
	casos := []struct {
		codigo string
		quer   bureau.Situacao
	}{
		{"0", bureau.Regular},
		{"2", bureau.Irregular},   // suspensa
		{"3", bureau.Falecido},    // titular falecido
		{"4", bureau.Irregular},   // pendente de regularização
		{"5", bureau.Inexistente}, // cancelada por multiplicidade
		{"8", bureau.Inexistente}, // nula
		{"9", bureau.Inexistente}, // cancelada de ofício
	}
	for _, c := range casos {
		corpo := strings.Replace(fixture(t, "cpf_40442820135_regular.json"), `"codigo":"0"`, `"codigo":"`+c.codigo+`"`, 1)
		r, err := consultar(t, servidor(t, responde(200, corpo)), cpfRegular, nascRegular)
		if err != nil || r.Situacao != c.quer {
			t.Errorf("código %s: situação %q, erro %v; quer %q", c.codigo, r.Situacao, err, c.quer)
		}
	}
}

func TestMenores(t *testing.T) {
	// 451 (menor de 16): o Serpro não devolve nada; vira documento_invalido.
	r, err := consultar(t, servidor(t, responde(451, `{}`)), "20803407009", "2021-07-16")
	if err != nil || r.Situacao != bureau.Inexistente {
		t.Fatalf("451: %+v, %v", r, err)
	}
	// 422 (menor de 18): devolve o nascimento informado para o corte MENOR_IDADE.
	r, err = consultar(t, servidor(t, responde(422, `{}`)), "30272095079", "2009-02-03")
	if err != nil || r.Nascimento != "2009-02-03" || r.Situacao != "" {
		t.Fatalf("422: %+v, %v", r, err)
	}
	// 422 com nascimento informado de adulto: não confere, recusa (não deixa o adulto inventado passar).
	for _, nasc := range []string{"2008-11-08", "1990-01-01"} { // 18 anos exatos hoje; bem adulto
		r, err = consultar(t, servidor(t, responde(422, `{}`)), "30272095079", nasc)
		if err != nil || r.Situacao != bureau.Inexistente {
			t.Fatalf("422 com %s: %+v, %v", nasc, r, err)
		}
	}
	// 17 anos e 364 dias: ainda menor, segue para o corte.
	r, err = consultar(t, servidor(t, responde(422, `{}`)), "30272095079", "2008-11-09")
	if err != nil || r.Nascimento != "2008-11-09" || r.Situacao != "" {
		t.Fatalf("422 na borda: %+v, %v", r, err)
	}
}

func TestParcial(t *testing.T) {
	r, err := consultar(t, servidor(t, responde(206, `{"ni":"88373571094","nome":"FULANO"}`)), "88373571094", "2000-02-02")
	quer := bureau.Resultado{Nome: "FULANO", Fontes: bureau.Fontes{Cadastro: bureau.FonteSerproTrial}}
	if err != nil || !reflect.DeepEqual(r, quer) {
		t.Fatalf("206: %+v, %v", r, err)
	}
}

func TestFailClosed(t *testing.T) {
	casos := []struct {
		nome string
		h    http.HandlerFunc
	}{
		{"429 cota", responde(429, fixture(t, "cota_estourada_429.json"))},
		{"401", responde(401, `{}`)},
		{"403", responde(403, `{}`)},
		{"400", responde(400, `{"code":"400"}`)},
		{"404", responde(404, `{}`)},
		{"500", responde(500, fixture(t, "cpf_40442820135_regular.json"))},
		{"503", responde(503, ``)},
		{"json inválido", responde(200, `{"situacao":`)},
		{"situação ausente no 200", responde(200, `{"nome":"X"}`)},
		{"situação desconhecida", responde(200, `{"situacao":{"codigo":"7"}}`)},
		{"situação desconhecida no 206", responde(206, `{"situacao":{"codigo":"x"}}`)},
		{"nascimento malformado", responde(200, `{"situacao":{"codigo":"0"},"nascimento":"1970-11-14"}`)},
		{"corpo gigante", responde(200, `{"nome":"`+strings.Repeat("A", 1<<20)+`","situacao":{"codigo":"0"}}`)},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r, err := consultar(t, servidor(t, c.h), cpfRegular, nascRegular)
			if !errors.Is(err, bureau.ErrIndisponivel) || !reflect.DeepEqual(r, bureau.Resultado{}) {
				t.Fatalf("resultado %+v, erro %v; quer ErrIndisponivel", r, err)
			}
			if strings.Contains(err.Error(), cpfRegular) || strings.Contains(err.Error(), "chave-teste") {
				t.Fatalf("erro expõe CPF ou chave: %v", err)
			}
		})
	}
}

func TestTimeoutDoCtx(t *testing.T) {
	c := servidor(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := c.ConsultarComNascimento(ctx, cpfRegular, nascRegular)
	if !errors.Is(err, bureau.ErrIndisponivel) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro = %v, quer ErrIndisponivel + DeadlineExceeded", err)
	}
	if strings.Contains(err.Error(), cpfRegular) || strings.Contains(err.Error(), "14111970") {
		t.Fatalf("erro expõe CPF/URL (vai para o log): %v", err)
	}
}

func TestRecusaSemChamar(t *testing.T) {
	chamou := false
	c := servidor(t, func(http.ResponseWriter, *http.Request) { chamou = true })
	casos := []struct{ doc, nasc string }{
		{"12345678900", nascRegular},    // dígito errado
		{"00000000000191", nascRegular}, // CNPJ
		{cpfRegular, ""},                // sem nascimento
		{cpfRegular, "14/11/1970"},      // nascimento malformado
	}
	for _, x := range casos {
		if _, err := c.ConsultarComNascimento(context.Background(), x.doc, x.nasc); err == nil {
			t.Errorf("%s/%q: quer erro", x.doc, x.nasc)
		}
	}
	if _, err := c.Consultar(context.Background(), cpfRegular); err == nil {
		t.Error("Consultar sem nascimento: quer erro")
	}
	if chamou {
		t.Fatal("chamou o Serpro com pedido que ele recusaria")
	}
}

// Ao vivo (gasta a cota mínima do trial; use com parcimônia):
// SERPRO_AO_VIVO=on go test ./internal/bureau/serpro -run AoVivo
func TestAoVivo(t *testing.T) {
	if os.Getenv("SERPRO_AO_VIVO") != "on" {
		t.Skip("defina SERPRO_AO_VIVO=on para chamar o trial do Serpro de verdade")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := Novo(URLTrial, ChaveTrial, true).(bureau.ComNascimento).ConsultarComNascimento(ctx, cpfRegular, nascRegular)
	if err != nil || r.Situacao != bureau.Regular || r.Nascimento != nascRegular {
		t.Fatalf("resultado %+v, erro %v", r, err)
	}
}
