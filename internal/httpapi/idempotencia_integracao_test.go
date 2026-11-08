package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/consulta"
	"github.com/romeroalmeida/motor-credito/internal/httpapi"
	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

// bureauContado conta as chamadas e pode atrasar, para forçar requisições simultâneas a se sobreporem.
type bureauContado struct {
	bureau.Bureau
	chamadas atomic.Int64
	atraso   atomic.Int64 // nanossegundos
}

func (b *bureauContado) Consultar(ctx context.Context, doc string) (bureau.Resultado, error) {
	b.chamadas.Add(1)
	select {
	case <-time.After(time.Duration(b.atraso.Load())):
	case <-ctx.Done():
		return bureau.Resultado{}, ctx.Err()
	}
	return b.Bureau.Consultar(ctx, doc)
}

// Achado 5: requisições simultâneas com a mesma Idempotency-Key e corpos diferentes. Nenhuma recebe a
// decisão do outro corpo, o bureau roda uma vez só e fica uma consulta só para a chave.
func TestIdempotenciaConcorrente(t *testing.T) {
	a := novoAmbiente(t)
	cont := &bureauContado{Bureau: bureau.NovoSimulado(0)}
	tenants := tenant.NovoServico(a.banco.App)
	a.h = httpapi.Novo(httpapi.Deps{
		Pool: a.banco.App, Tenants: tenants,
		Auth:      painelauth.Novo(a.banco.App, tenants, painelauth.Opcoes{ChaveCifra: chaveCifra, CookieInseguro: true}),
		Consultas: consulta.Novo(a.banco.App, cont, chaveHMAC, chaveCifra),
	})
	a.dono(`UPDATE tenants SET limite_por_minuto = 100000 WHERE id = $1`, a.tenantA)
	cont.atraso.Store(int64(300 * time.Millisecond))
	defer cont.atraso.Store(0)
	const n = 12
	valores := [2]int64{400_000, 500_000}
	for rodada := range 3 {
		chave := "corrida-" + strconv.Itoa(rodada)
		cont.chamadas.Store(0)
		var wg sync.WaitGroup
		respostas := make([]*httptest.ResponseRecorder, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				respostas[i] = a.api(corpoConsulta(bureau.CPFScoreAlto, valores[i%2]), chave, true)
			}()
		}
		wg.Wait()
		vencedor := int64(0)
		for i, w := range respostas {
			meu := valores[i%2]
			switch w.Code {
			case http.StatusOK:
				var r consulta.Resposta
				if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
					t.Fatal(err)
				}
				if r.Pedido.Valor != meu {
					t.Fatalf("requisição de %d recebeu a decisão do pedido de %d", meu, r.Pedido.Valor)
				}
				if vencedor != 0 && vencedor != meu {
					t.Fatalf("os dois corpos receberam 200 para a mesma chave")
				}
				vencedor = meu
			case http.StatusConflict:
				if !strings.Contains(w.Body.String(), "conflito_idempotencia") {
					t.Fatalf("409 sem conflito_idempotencia: %s", w.Body.String())
				}
			default:
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		}
		if vencedor == 0 {
			t.Fatal("nenhuma requisição recebeu 200")
		}
		for i, w := range respostas {
			if (w.Code == http.StatusOK) != (valores[i%2] == vencedor) {
				t.Fatalf("requisição %d (pedido %d, vencedor %d) com status %d", i, valores[i%2], vencedor, w.Code)
			}
		}
		// Sem o cache de idempotência (como quando o perdedor guarda antes do vencedor, ou a gravação do cache
		// falha), a trava da consulta sozinha precisa recusar o outro corpo e reaproveitar o mesmo.
		a.dono(`DELETE FROM idempotencia WHERE tenant_id = $1 AND chave = $2`, a.tenantA, chave)
		outro := valores[0] + valores[1] - vencedor
		if w := a.api(corpoConsulta(bureau.CPFScoreAlto, outro), chave, true); w.Code != http.StatusConflict {
			t.Fatalf("outro corpo sem cache: %d %s", w.Code, w.Body.String())
		}
		if r := decodificar[consulta.Resposta](t, a.api(corpoConsulta(bureau.CPFScoreAlto, vencedor), chave, true), http.StatusOK); r.Pedido.Valor != vencedor {
			t.Fatalf("mesmo corpo sem cache recebeu pedido %d", r.Pedido.Valor)
		}
		if c := cont.chamadas.Load(); c != 1 {
			t.Fatalf("bureau chamado %d vezes para a chave %s", c, chave)
		}
		if c := a.contar(`SELECT count(*) FROM consultas WHERE idempotency_key = $1`, chave); c != 1 {
			t.Fatalf("%d consultas para a chave %s", c, chave)
		}
	}
	// Consulta gravada antes da 0003 (hash nulo): sem como comparar o corpo, o reenvio é conflito.
	a.dono(`UPDATE consultas SET idempotency_hash = NULL WHERE tenant_id = $1 AND idempotency_key = 'corrida-0'`, a.tenantA)
	a.dono(`DELETE FROM idempotencia WHERE tenant_id = $1 AND chave = 'corrida-0'`, a.tenantA)
	for _, v := range valores {
		if w := a.api(corpoConsulta(bureau.CPFScoreAlto, v), "corrida-0", true); w.Code != http.StatusConflict {
			t.Fatalf("hash nulo, pedido %d: %d %s", v, w.Code, w.Body.String())
		}
	}
}
