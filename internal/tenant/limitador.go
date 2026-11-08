package tenant

import (
	"sync"
	"time"
)

// limpezaAcima é o tamanho do mapa a partir do qual janelas vencidas são descartadas, no máximo uma vez por janela:
// varrer a cada chamada deixaria cada Permitir O(n) quando há mais de limpezaAcima chaves vivas.
const limpezaAcima = 10_000

// Limitador conta eventos por chave em janela fixa.
// shortcut: em memória e por processo; com mais de uma instância, trocar por Redis ou Postgres.
type Limitador struct {
	mu            sync.Mutex
	janelas       map[string]janela
	ultimaLimpeza time.Time
	limpezas      int // para teste
}

type janela struct {
	inicio time.Time
	usos   int
}

// NovoLimitador cria um limitador vazio.
func NovoLimitador() *Limitador { return &Limitador{janelas: map[string]janela{}} }

// Permitir registra um uso de chave. Devolve false e quanto falta para a janela virar quando passou de limite.
func (l *Limitador) Permitir(chave string, limite int, duracao time.Duration, agora time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.janelas) > limpezaAcima && agora.Sub(l.ultimaLimpeza) >= duracao {
		l.limpar(duracao, agora)
	}
	j := l.janelas[chave]
	if agora.Sub(j.inicio) >= duracao {
		j = janela{inicio: agora}
	}
	if j.usos >= limite {
		return false, j.inicio.Add(duracao).Sub(agora)
	}
	j.usos++
	l.janelas[chave] = j
	return true, 0
}

func (l *Limitador) limpar(duracao time.Duration, agora time.Time) {
	l.ultimaLimpeza = agora
	l.limpezas++
	for k, j := range l.janelas {
		if agora.Sub(j.inicio) >= duracao {
			delete(l.janelas, k)
		}
	}
}
