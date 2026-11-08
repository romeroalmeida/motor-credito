package bureau

import "time"

// Score cadastral de CNPJ (0..1000) a partir só de dados públicos da Receita (BrasilAPI). Soma de sete fatores;
// cada um devolve um código e os pontos, para a decisão ser explicável. Tabela no README do bureau.
// O CNAE fica de fora: risco por setor exige base estatística que não temos, e uma lista feita à mão não se defende.
const (
	PontosSituacaoAtiva = 250

	PontosTempoMenos1A = 0
	PontosTempo1A      = 80 // 1 a 2 anos completos
	PontosTempo3A      = 140
	PontosTempo5AMais  = 200

	CapitalFaixa1 = 10_000_00    // R$ 10 mil, em centavos
	CapitalFaixa2 = 100_000_00   // R$ 100 mil
	CapitalFaixa3 = 1_000_000_00 // R$ 1 milhão

	PontosCapitalAte10Mil  = 0
	PontosCapital10Mil     = 50
	PontosCapital100Mil    = 100
	PontosCapital1MiMais   = 150
	PontosPorteMicro       = 40
	PontosPortePequeno     = 70
	PontosPorteDemais      = 100
	PontosRegimeMEI        = 0
	PontosRegimeSimples    = 50
	PontosRegimeNormal     = 100 // fora do Simples e do MEI: faturamento acima do teto ou opção pelo lucro presumido/real
	PontosNaturezaSA       = 150
	PontosNaturezaLimitada = 100
	PontosNaturezaIndiv    = 30
	PontosSocio1           = 20
	PontosSocios2Mais      = 50

	// Códigos de porte da Receita.
	PorteMicro   = 1
	PortePequeno = 3
	PorteDemais  = 5
)

// Naturezas jurídicas (tabela da Receita/Concla) que pontuam; as demais (associação, fundação, órgão público…) dão 0.
var naturezas = map[int]Fator{
	2011: {"NATUREZA_SA", PontosNaturezaSA}, // empresa pública
	2038: {"NATUREZA_SA", PontosNaturezaSA}, // sociedade de economia mista
	2046: {"NATUREZA_SA", PontosNaturezaSA}, // S.A. aberta
	2054: {"NATUREZA_SA", PontosNaturezaSA}, // S.A. fechada
	2062: {"NATUREZA_LIMITADA", PontosNaturezaLimitada},
	2135: {"NATUREZA_INDIVIDUAL", PontosNaturezaIndiv}, // empresário individual (inclui MEI)
	2305: {"NATUREZA_INDIVIDUAL", PontosNaturezaIndiv}, // EIRELI empresária
	2313: {"NATUREZA_INDIVIDUAL", PontosNaturezaIndiv}, // EIRELI simples
}

// ScoreCadastral é puro e determinístico: mesma empresa e mesmo dia, mesmo score e mesmos fatores, nesta ordem.
// abertura "" ou malformada conta como tempo desconhecido (0 pontos).
func ScoreCadastral(situacao Situacao, abertura string, e Empresa, hoje time.Time) (int, []Fator) {
	fatores := []Fator{
		fatorSituacao(situacao),
		fatorTempo(abertura, hoje),
		fatorCapital(e.CapitalSocial),
		fatorPorte(e.CodigoPorte),
		fatorRegime(e.Simples, e.MEI),
		fatorNatureza(e.Natureza),
		fatorSocios(e.Socios),
	}
	total := 0
	for _, f := range fatores {
		total += f.Pontos
	}
	return total, fatores // pontos não negativos e topos somando 1000: fica em 0..1000 (o teste do topo prova)
}

func fatorSituacao(s Situacao) Fator {
	if s == Regular {
		return Fator{"SITUACAO_ATIVA", PontosSituacaoAtiva}
	}
	return Fator{"SITUACAO_INATIVA", 0}
}

func fatorTempo(abertura string, hoje time.Time) Fator {
	anos, ok := idadeCompleta(abertura, hoje)
	switch {
	case !ok:
		return Fator{"TEMPO_EMPRESA_DESCONHECIDO", 0}
	case anos >= 5:
		return Fator{"TEMPO_EMPRESA_5A_MAIS", PontosTempo5AMais}
	case anos >= 3:
		return Fator{"TEMPO_EMPRESA_3A_5A", PontosTempo3A}
	case anos >= 1:
		return Fator{"TEMPO_EMPRESA_1A_3A", PontosTempo1A}
	}
	return Fator{"TEMPO_EMPRESA_MENOS_1A", PontosTempoMenos1A}
}

// idadeCompleta: anos completos de abertura até hoje; abertura no futuro ou malformada = ok false.
func idadeCompleta(data string, hoje time.Time) (int, bool) {
	d, err := time.Parse(time.DateOnly, data)
	if err != nil {
		return 0, false
	}
	anos := hoje.Year() - d.Year()
	if hoje.Month() < d.Month() || (hoje.Month() == d.Month() && hoje.Day() < d.Day()) {
		anos--
	}
	return anos, anos >= 0
}

func fatorCapital(centavos int64) Fator {
	switch {
	case centavos >= CapitalFaixa3:
		return Fator{"CAPITAL_1MI_MAIS", PontosCapital1MiMais}
	case centavos >= CapitalFaixa2:
		return Fator{"CAPITAL_100MIL_1MI", PontosCapital100Mil}
	case centavos >= CapitalFaixa1:
		return Fator{"CAPITAL_10MIL_100MIL", PontosCapital10Mil}
	}
	return Fator{"CAPITAL_ATE_10MIL", PontosCapitalAte10Mil}
}

func fatorPorte(codigo int) Fator {
	switch codigo {
	case PorteMicro:
		return Fator{"PORTE_MICRO", PontosPorteMicro}
	case PortePequeno:
		return Fator{"PORTE_PEQUENO", PontosPortePequeno}
	case PorteDemais:
		return Fator{"PORTE_DEMAIS", PontosPorteDemais}
	}
	return Fator{"PORTE_NAO_INFORMADO", 0}
}

func fatorRegime(simples, mei bool) Fator {
	switch {
	case mei: // o MEI também é optante do Simples; o MEI vale primeiro
		return Fator{"REGIME_MEI", PontosRegimeMEI}
	case simples:
		return Fator{"REGIME_SIMPLES", PontosRegimeSimples}
	}
	return Fator{"REGIME_NORMAL", PontosRegimeNormal}
}

func fatorNatureza(codigo int) Fator {
	if f, ok := naturezas[codigo]; ok {
		return f
	}
	return Fator{"NATUREZA_OUTRA", 0}
}

func fatorSocios(n int) Fator {
	switch {
	case n >= 2:
		return Fator{"SOCIOS_2_MAIS", PontosSocios2Mais}
	case n == 1:
		return Fator{"SOCIO_1", PontosSocio1}
	}
	return Fator{"SOCIOS_NAO_INFORMADO", 0}
}
