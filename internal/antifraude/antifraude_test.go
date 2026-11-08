package antifraude

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
)

// contadorFake é o Contador em memória; devolve ctx.Err() para provar que o ctx chega até ele.
type contadorFake struct {
	consultasDoc, docsDispositivo int
	err                           error
}

func (f contadorFake) ConsultasDoc24h(ctx context.Context, _ []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return f.consultasDoc, f.err
}

func (f contadorFake) DocsPorDispositivo24h(ctx context.Context, _ string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return f.docsDispositivo, f.err
}

// entradaLimpa tem todos os dados, todos coerentes: executa as 7 checagens sem sinal.
func entradaLimpa() Entrada {
	return Entrada{
		DocHash: []byte{1, 2, 3}, Tipo: "cpf",
		Nome: "Joana Pereira", Nascimento: bureau.NascimentoFixture,
		Email: "joana@exemplo.com.br", IP: "177.10.20.30", DeviceID: "disp-1",
		Bureau: bureau.Resultado{Nome: bureau.NomeFixture, Nascimento: bureau.NascimentoFixture, Situacao: bureau.Regular},
	}
}

var todas = []string{"documento", "velocidade", "nome", "nascimento", "email", "ip", "dispositivo"}

func avaliar(t *testing.T, e Entrada, c Contador) Resultado {
	t.Helper()
	r, err := Avaliar(context.Background(), e, c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestEntradaLimpa(t *testing.T) {
	r := avaliar(t, entradaLimpa(), contadorFake{})
	if r.Risco != 0 || len(r.Sinais) != 0 || !slices.Equal(r.Executadas, todas) || len(r.Puladas) != 0 {
		t.Fatalf("resultado %+v", r)
	}
}

func TestChecagemPuladaQuandoDadoFalta(t *testing.T) {
	casos := []struct {
		pulada string
		tirar  func(*Entrada)
	}{
		{"documento", func(e *Entrada) { e.Bureau.Situacao = "" }},
		{"velocidade", func(e *Entrada) { e.DocHash = nil }},
		{"nome", func(e *Entrada) { e.Nome = "" }},
		{"nome", func(e *Entrada) { e.Bureau.Nome = "" }},
		{"nome", func(e *Entrada) { e.Nome = "   " }},
		{"nascimento", func(e *Entrada) { e.Nascimento = "" }},
		{"nascimento", func(e *Entrada) { e.Bureau.Nascimento = "" }},
		{"email", func(e *Entrada) { e.Email = "" }},
		{"email", func(e *Entrada) { e.Email = "sem-arroba" }},
		{"ip", func(e *Entrada) { e.IP = "" }},
		{"ip", func(e *Entrada) { e.IP = "não-é-ip" }},
		{"ip", func(e *Entrada) { e.IP = "192.168.0.10" }},
		{"ip", func(e *Entrada) { e.IP = "127.0.0.1" }},
		{"dispositivo", func(e *Entrada) { e.DeviceID = "" }},
	}
	for _, c := range casos {
		e := entradaLimpa()
		c.tirar(&e)
		// contador que acusaria tudo: se a checagem rodasse, apareceria sinal
		r := avaliar(t, e, contadorFake{consultasDoc: 99, docsDispositivo: 99})
		if !slices.Equal(r.Puladas, []string{c.pulada}) || slices.Contains(r.Executadas, c.pulada) {
			t.Errorf("%s: puladas %v executadas %v", c.pulada, r.Puladas, r.Executadas)
		}
	}
}

func TestSemNenhumDado(t *testing.T) {
	r := avaliar(t, Entrada{Tipo: "cpf"}, nil)
	if r.Risco != 0 || len(r.Executadas) != 0 || !slices.Equal(r.Puladas, todas) {
		t.Fatalf("resultado %+v", r)
	}
}

func TestVelocidadeNoLimite(t *testing.T) {
	casos := []struct {
		doc, disp int
		sinais    []string
	}{
		{LimiteConsultasDoc24h, LimiteDocsPorDispositivo24h, nil},
		{LimiteConsultasDoc24h + 1, LimiteDocsPorDispositivo24h, []string{SinalVelocidadeAlta}},
		{LimiteConsultasDoc24h, LimiteDocsPorDispositivo24h + 1, []string{SinalDispositivoMultiplosCPFs}},
	}
	for _, c := range casos {
		r := avaliar(t, entradaLimpa(), contadorFake{consultasDoc: c.doc, docsDispositivo: c.disp})
		if !slices.Equal(r.Sinais, c.sinais) {
			t.Errorf("doc=%d disp=%d: sinais %v, quer %v", c.doc, c.disp, r.Sinais, c.sinais)
		}
	}
}

func TestSituacaoDocumento(t *testing.T) {
	casos := []struct {
		tipo     string
		situacao bureau.Situacao
		sinais   []string
	}{
		{"cpf", bureau.Regular, nil},
		{"cpf", bureau.Irregular, []string{SinalCPFIrregular}},
		{"cpf", bureau.Inativo, []string{SinalCPFIrregular}},
		{"cpf", bureau.Falecido, nil}, // E trata como documento_invalido
		{"cnpj", bureau.Inativo, []string{SinalCNPJInativo}},
		{"cnpj", bureau.Irregular, []string{SinalCNPJInativo}},
	}
	for _, c := range casos {
		e := entradaLimpa()
		e.Tipo, e.Bureau.Situacao = c.tipo, c.situacao
		r := avaliar(t, e, contadorFake{})
		if !slices.Equal(r.Sinais, c.sinais) || !slices.Contains(r.Executadas, "documento") {
			t.Errorf("%s/%s: sinais %v", c.tipo, c.situacao, r.Sinais)
		}
	}
}

func TestNome(t *testing.T) {
	casos := []struct {
		informado, doBureau string
		divergente          bool
	}{
		{"Joana Pereira", "JOANA PEREIRA DOS SANTOS", false},
		{"joana santos", "JOANA PEREIRA DOS SANTOS", false},
		{"Maria da Conceição", "MARIA CONCEICAO", false}, // acento e preposição
		{"JOSÉ D'ÁVILA", "Jose Avila", false},            // apóstrofo e caixa
		{"Joana Lima", "JOANA PEREIRA DOS SANTOS", true},
		{"Ana de Oliveira", "ANA DE SOUZA", true}, // preposição comum não salva o sobrenome
		{"Pereira", "JOANA PEREIRA", false},
		{"Joana", "JOANA PEREIRA", true}, // primeiro nome não é sobrenome
	}
	for _, c := range casos {
		if got := nomeDivergente(c.informado, c.doBureau); got != c.divergente {
			t.Errorf("%q × %q = %v, quer %v", c.informado, c.doBureau, got, c.divergente)
		}
	}
}

func TestNascimento(t *testing.T) {
	e := entradaLimpa()
	e.Nascimento = "1985-04-13"
	if r := avaliar(t, e, contadorFake{}); !slices.Equal(r.Sinais, []string{SinalNascimentoDivergente}) {
		t.Fatalf("sinais %v", r.Sinais)
	}
}

func TestEmail(t *testing.T) {
	casos := map[string]bool{
		"x@mailinator.com":    true,
		"X@MailInator.COM":    true,
		"a@b@yopmail.com":     true,
		"joana@gmail.com":     false,
		"x@mailinator.com.br": false,
	}
	for email, descartavel := range casos {
		e := entradaLimpa()
		e.Email = email
		r := avaliar(t, e, contadorFake{})
		if slices.Contains(r.Sinais, SinalEmailDescartavel) != descartavel {
			t.Errorf("%s: sinais %v", email, r.Sinais)
		}
	}
}

func TestIP(t *testing.T) {
	casos := []struct {
		ip     string
		sinais []string
	}{
		{"177.10.20.30", nil},
		{"::ffff:201.1.2.3", nil},
		{"2804:14c:1::1", nil},
		{"8.8.8.8", []string{SinalIPEstrangeiro}},
		{"54.10.20.30", []string{SinalIPEstrangeiro, SinalIPDatacenter}},
		{"2600:1f00::1", []string{SinalIPEstrangeiro, SinalIPDatacenter}},
	}
	for _, c := range casos {
		e := entradaLimpa()
		e.IP = c.ip
		r := avaliar(t, e, contadorFake{})
		if !slices.Equal(r.Sinais, c.sinais) {
			t.Errorf("%s: sinais %v, quer %v", c.ip, r.Sinais, c.sinais)
		}
	}
}

func TestRiscoSomaELimita(t *testing.T) {
	e := entradaLimpa()
	e.Email = "x@mailinator.com"
	e.IP = "8.8.8.8"
	r := avaliar(t, e, contadorFake{})
	if r.Risco != PesoEmailDescartavel+PesoIPEstrangeiro {
		t.Errorf("risco %d, quer soma %d", r.Risco, PesoEmailDescartavel+PesoIPEstrangeiro)
	}

	e = entradaLimpa()
	e.Bureau.Situacao = bureau.Irregular
	e.Nome, e.Nascimento, e.Email, e.IP = "Joana Lima", "2000-01-01", "x@mailinator.com", "54.1.1.1"
	r = avaliar(t, e, contadorFake{consultasDoc: 99, docsDispositivo: 99})
	if len(r.Sinais) != 8 || r.Risco != RiscoMaximo {
		t.Errorf("todos os sinais: %v risco %d, quer %d", r.Sinais, r.Risco, RiscoMaximo)
	}
}

func TestErroDoContadorAborta(t *testing.T) {
	falha := errors.New("banco fora")
	if _, err := Avaliar(context.Background(), entradaLimpa(), contadorFake{err: falha}); !errors.Is(err, falha) {
		t.Fatalf("erro = %v, quer %v", err, falha)
	}
}

func TestRespeitaCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Avaliar(ctx, entradaLimpa(), contadorFake{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, quer context.Canceled", err)
	}
}

func TestComBureauSimulado(t *testing.T) {
	res, err := bureau.NovoSimulado(0).Consultar(context.Background(), bureau.CPFNomeConhecido)
	if err != nil {
		t.Fatal(err)
	}
	e := Entrada{DocHash: []byte{9}, Tipo: "cpf", Nome: "Joana Lima", Bureau: res}
	r := avaliar(t, e, contadorFake{})
	if !slices.Equal(r.Sinais, []string{SinalNomeDivergente}) || r.Risco != PesoNomeDivergente {
		t.Fatalf("resultado %+v", r)
	}
}
