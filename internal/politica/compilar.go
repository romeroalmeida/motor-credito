package politica

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common"
	"cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/cost"

	"github.com/romeroalmeida/motor-credito/internal/dinheiro"
)

// Limites de cada expressão. Valem na publicação (estimativa estática) e na avaliação (CostLimit).
const (
	CustoMaximo      = 100  // unidades de custo CEL por expressão
	TamanhoMaximo    = 2000 // pontos de código por expressão
	maxSinais        = 16   // tamanho máximo de fraude.sinais assumido pela estimativa de custo
	maxTamanhoSinal  = 64   // tamanho máximo de cada sinal assumido pela estimativa de custo
	maxMultiploRenda = 1000 // 100× a renda
	recursaoMaxima   = 32
	identOpcional    = "opcional" // mapa oculto que o has() consulta; o cliente não o referencia
	campoSinais      = "fraude.sinais"
	expressaoVerdade = "true"
	operadorPertence = "in"
)

// camposInteiros são as variáveis int do ambiente; renda e idade são opcionais e exigem has().
var camposInteiros = []string{
	"score", "bureau.restricoes", "bureau.protestos", "bureau.divida_total",
	"renda", "idade", "pedido.valor", "pedido.prazo", "fraude.risco",
}

var opcionais = map[string]bool{"renda": true, "idade": true}

var operadoresVisuais = map[string]bool{"==": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true, operadorPertence: true}

// funcoesPermitidas é a lista fechada de funções CEL que uma expressão pode chamar.
// Qualquer outra (size, matches, string, timestamp, métodos...) é recusada, mesmo existindo na biblioteca padrão.
var funcoesPermitidas = map[string]bool{
	"_==_": true, "_!=_": true, "_<_": true, "_<=_": true, "_>_": true, "_>=_": true,
	"@in": true, "_&&_": true, "_||_": true, "!_": true, "_?_:_": true,
	"_+_": true, "_-_": true, "_*_": true, "_/_": true, "_%_": true, "-_": true,
}

// Problema aponta uma regra que não compilou: Regra no formato "faixas[2].condicoes[1]".
type Problema struct {
	Regra  string `json:"regra"`
	Motivo string `json:"motivo"`
}

// ErroPolitica lista todos os problemas encontrados por Compilar.
type ErroPolitica struct{ Problemas []Problema }

func (e *ErroPolitica) Error() string {
	partes := make([]string, len(e.Problemas))
	for i, p := range e.Problemas {
		partes[i] = p.Regra + ": " + p.Motivo
	}
	return "política inválida: " + strings.Join(partes, "; ")
}

// regraCompilada é uma condição pronta para avaliar.
type regraCompilada struct {
	expressao string
	programa  cel.Program
}

// Compilada guarda os programas CEL prontos, na ordem da política.
type Compilada struct {
	politica Politica
	cortes   []regraCompilada
	faixas   []regraCompilada
}

var ambiente = novoAmbiente()

func novoAmbiente() *cel.Env {
	opcoes := []cel.EnvOption{
		cel.ClearMacros(), // sem exists/all/map/filter: nada de laço em expressão do cliente
		cel.Macros(cel.GlobalMacro("has", 1, expandirHas)),
		cel.ParserExpressionSizeLimit(TamanhoMaximo),
		cel.ParserRecursionLimit(recursaoMaxima),
		cel.Variable(campoSinais, cel.ListType(cel.StringType)),
		cel.Variable(identOpcional, cel.MapType(cel.StringType, cel.IntType)),
	}
	for _, campo := range camposInteiros {
		opcoes = append(opcoes, cel.Variable(campo, cel.IntType))
	}
	env, err := cel.NewEnv(opcoes...)
	if err != nil {
		panic(fmt.Sprintf("politica: ambiente CEL inválido: %v", err)) // erro de programação, não de entrada
	}
	return env
}

// expandirHas troca has(renda) por has(opcional.renda): o has() padrão do CEL não aceita identificador solto.
func expandirHas(eh cel.MacroExprFactory, _ ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
	arg := args[0]
	if arg.Kind() != ast.IdentKind || !opcionais[arg.AsIdent()] {
		return nil, eh.NewError(arg.ID(), "has() só aceita renda ou idade")
	}
	return eh.NewPresenceTest(eh.NewIdent(identOpcional), arg.AsIdent()), nil
}

// Compilar valida a política inteira e devolve os programas prontos. O erro (*ErroPolitica) lista cada regra e o motivo.
func Compilar(p Politica) (*Compilada, error) {
	c := &Compilada{politica: p}
	var problemas []Problema
	anotar := func(regra string, err error) {
		problemas = append(problemas, Problema{Regra: regra, Motivo: err.Error()})
	}

	if len(p.Faixas) == 0 {
		anotar("faixas", errors.New("a política precisa de ao menos uma faixa"))
	}
	if err := validarContraproposta(p.Contraproposta); err != nil {
		anotar("contraproposta", err)
	}
	for i, corte := range p.Cortes {
		regra := fmt.Sprintf("cortes[%d]", i)
		if corte.Codigo == "" {
			anotar(regra, errors.New("corte sem código"))
		}
		r, err := compilarRegra(regra, corte.Condicoes, corte.CEL, p.ExpressaoAvancada, false)
		if err != nil {
			anotar(regra, err)
		}
		c.cortes = append(c.cortes, r)
	}
	for i, faixa := range p.Faixas {
		regra := fmt.Sprintf("faixas[%d]", i)
		if err := validarFaixa(faixa); err != nil {
			anotar(regra, err)
		}
		r, err := compilarRegra(regra, faixa.Condicoes, faixa.CEL, p.ExpressaoAvancada, true)
		if err != nil {
			anotar(regra, err)
		}
		c.faixas = append(c.faixas, r)
	}

	if len(problemas) > 0 {
		return nil, &ErroPolitica{Problemas: problemas}
	}
	return c, nil
}

func validarContraproposta(cp Contraproposta) error {
	if cp.PrazoMaximo < 0 || cp.AcrescimoTaxaBps < 0 {
		return errors.New("prazo máximo e acréscimo de taxa não podem ser negativos")
	}
	return nil
}

func validarFaixa(f Faixa) error {
	switch {
	case f.Rotulo == "":
		return errors.New("faixa sem rótulo")
	case f.Nega:
		return nil
	case f.TaxaBps < 0 || f.LimiteFixo < 0 || f.MultiploRenda < 0:
		return errors.New("taxa, limite e múltiplo não podem ser negativos")
	case f.LimiteFixo == 0:
		return errors.New("faixa que aprova precisa de limite fixo maior que zero")
	case f.LimiteFixo > dinheiro.ValorMaximo:
		return fmt.Errorf("limite fixo acima de %d centavos", dinheiro.ValorMaximo)
	case f.MultiploRenda > maxMultiploRenda:
		return fmt.Errorf("múltiplo de renda acima de %d", maxMultiploRenda)
	}
	return nil
}

// compilarRegra escolhe a origem da expressão (CEL do cliente ou condições visuais) e a compila.
// vazioCasa diz se uma regra sem condição casa com tudo (faixa "demais casos") ou é erro (corte).
func compilarRegra(regra string, condicoes []Condicao, expressaoCliente string, avancada, vazioCasa bool) (regraCompilada, error) {
	expressao := expressaoCliente
	if !avancada {
		gerada, err := condicoesParaCEL(regra, condicoes, vazioCasa)
		if err != nil {
			return regraCompilada{}, err
		}
		expressao = gerada
	}
	if strings.TrimSpace(expressao) == "" {
		return regraCompilada{}, errors.New("expressão vazia")
	}
	programa, err := compilarExpressao(expressao)
	if err != nil {
		return regraCompilada{}, err
	}
	return regraCompilada{expressao: expressao, programa: programa}, nil
}

// compilarExpressao passa pelo parser e pelo checker de tipos, confere a lista de funções e o has(),
// estima o custo e só então gera o programa com o limite de custo em tempo de execução.
func compilarExpressao(expressao string) (cel.Program, error) {
	checada, iss := ambiente.Compile(expressao)
	if iss.Err() != nil {
		return nil, iss.Err()
	}
	if checada.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("a expressão precisa resultar em verdadeiro/falso, resulta em %s", checada.OutputType())
	}
	if err := validarArvore(checada.NativeRep().Expr(), nil); err != nil {
		return nil, err
	}
	estimativa, err := ambiente.EstimateCost(checada, estimadorTamanho{})
	if err != nil {
		return nil, fmt.Errorf("custo não estimável: %w", err)
	}
	if estimativa.Max > CustoMaximo {
		return nil, fmt.Errorf("custo estimado %d acima do limite %d", estimativa.Max, CustoMaximo)
	}
	return ambiente.Program(checada, cel.CostLimit(CustoMaximo))
}

// validarArvore percorre a expressão já checada: só funções da lista e renda/idade só depois de has().
// protegidos são os opcionais garantidos pelo lado esquerdo de um && ou pela condição de um ternário.
func validarArvore(e ast.Expr, protegidos map[string]bool) error {
	switch e.Kind() {
	case ast.IdentKind:
		return validarIdent(e.AsIdent(), protegidos)
	case ast.SelectKind:
		if campo, ok := testePresenca(e); ok && opcionais[campo] {
			return nil
		}
		return validarArvore(e.AsSelect().Operand(), protegidos)
	case ast.CallKind:
		return validarChamada(e.AsCall(), protegidos)
	case ast.ListKind:
		for _, el := range e.AsList().Elements() {
			if err := validarArvore(el, protegidos); err != nil {
				return err
			}
		}
		return nil
	case ast.LiteralKind:
		return nil
	default: // mapas, structs e laços não têm uso em regra de crédito
		return errors.New("construção não permitida na expressão")
	}
}

func validarIdent(nome string, protegidos map[string]bool) error {
	if nome == identOpcional {
		return fmt.Errorf("campo desconhecido: %s", nome)
	}
	if opcionais[nome] && !protegidos[nome] {
		return fmt.Errorf("%s usada sem has(%s)", nome, nome)
	}
	return nil
}

func validarChamada(chamada ast.CallExpr, protegidos map[string]bool) error {
	nome := chamada.FunctionName()
	if chamada.IsMemberFunction() || !funcoesPermitidas[nome] {
		return fmt.Errorf("função não permitida: %s", strings.Trim(nome, "_@"))
	}
	args := chamada.Args()
	switch nome {
	case "_&&_":
		if err := validarArvore(args[0], protegidos); err != nil {
			return err
		}
		return validarArvore(args[1], comPresencas(protegidos, args[0]))
	case "_?_:_":
		for i, arg := range args {
			alcance := protegidos
			if i == 1 {
				alcance = comPresencas(protegidos, args[0])
			}
			if err := validarArvore(arg, alcance); err != nil {
				return err
			}
		}
		return nil
	}
	for _, arg := range args {
		if err := validarArvore(arg, protegidos); err != nil {
			return err
		}
	}
	return nil
}

// comPresencas soma aos protegidos os has() que valem sempre que guarda é verdadeira (conjunções de has()).
func comPresencas(protegidos map[string]bool, guarda ast.Expr) map[string]bool {
	novos := map[string]bool{}
	for k := range protegidos {
		novos[k] = true
	}
	var coletar func(ast.Expr)
	coletar = func(e ast.Expr) {
		if campo, ok := testePresenca(e); ok {
			novos[campo] = true
			return
		}
		if e.Kind() == ast.CallKind && e.AsCall().FunctionName() == "_&&_" {
			for _, a := range e.AsCall().Args() {
				coletar(a)
			}
		}
	}
	coletar(guarda)
	return novos
}

func testePresenca(e ast.Expr) (string, bool) {
	if e.Kind() != ast.SelectKind || !e.AsSelect().IsTestOnly() {
		return "", false
	}
	op := e.AsSelect().Operand()
	if op.Kind() != ast.IdentKind || op.AsIdent() != identOpcional {
		return "", false
	}
	return e.AsSelect().FieldName(), true
}

// estimadorTamanho informa o tamanho máximo de fraude.sinais, sem o qual o custo de "in" seria ilimitado.
type estimadorTamanho struct{}

func (estimadorTamanho) EstimateSize(no cost.AstNode) *cost.SizeEstimate {
	caminho := strings.Join(no.Path(), ".")
	switch caminho {
	case campoSinais:
		return &cost.SizeEstimate{Min: 0, Max: maxSinais}
	case campoSinais + ".@items":
		return &cost.SizeEstimate{Min: 0, Max: maxTamanhoSinal}
	}
	return nil
}

func (estimadorTamanho) EstimateCallCost(_, _ string, _ *cost.AstNode, _ []cost.AstNode) *cost.CallEstimate {
	return nil
}

// condicoesParaCEL monta a expressão do editor visual: condições ligadas por &&.
func condicoesParaCEL(regra string, condicoes []Condicao, vazioCasa bool) (string, error) {
	if len(condicoes) == 0 {
		if vazioCasa {
			return expressaoVerdade, nil
		}
		return "", errors.New("regra sem condição")
	}
	partes := make([]string, len(condicoes))
	for i, c := range condicoes {
		parte, err := condicaoParaCEL(c)
		if err != nil {
			return "", fmt.Errorf("condicoes[%d]: %w", i, err)
		}
		partes[i] = parte
	}
	return strings.Join(partes, " && "), nil
}

func condicaoParaCEL(c Condicao) (string, error) {
	if !operadoresVisuais[c.Operador] {
		return "", fmt.Errorf("operador não permitido: %q", c.Operador)
	}
	if c.Campo == campoSinais {
		return condicaoSinais(c)
	}
	if !campoInteiroConhecido(c.Campo) {
		return "", fmt.Errorf("campo desconhecido: %q", c.Campo)
	}
	valor, err := valorInteiroCEL(c.Valor, c.Operador == operadorPertence)
	if err != nil {
		return "", err
	}
	expressao := fmt.Sprintf("%s %s %s", c.Campo, c.Operador, valor)
	if opcionais[c.Campo] {
		expressao = fmt.Sprintf("(has(%s) && %s)", c.Campo, expressao)
	}
	return expressao, nil
}

// condicaoSinais: "in" com um sinal ou uma lista (basta um estar presente).
func condicaoSinais(c Condicao) (string, error) {
	if c.Operador != operadorPertence {
		return "", fmt.Errorf("fraude.sinais só aceita o operador in")
	}
	sinais, err := textos(c.Valor)
	if err != nil {
		return "", err
	}
	partes := make([]string, len(sinais))
	for i, s := range sinais {
		partes[i] = strconv.Quote(s) + " in " + campoSinais
	}
	return "(" + strings.Join(partes, " || ") + ")", nil
}

func campoInteiroConhecido(campo string) bool {
	for _, c := range camposInteiros {
		if c == campo {
			return true
		}
	}
	return false
}

// valorInteiroCEL aceita o número que o JSON decodificou (float64 inteiro) ou tipos inteiros de Go.
func valorInteiroCEL(valor any, lista bool) (string, error) {
	if !lista {
		n, err := inteiro(valor)
		return strconv.FormatInt(n, 10), err
	}
	itens, ok := valor.([]any)
	if !ok || len(itens) == 0 {
		return "", errors.New("o operador in precisa de uma lista não vazia")
	}
	partes := make([]string, len(itens))
	for i, item := range itens {
		n, err := inteiro(item)
		if err != nil {
			return "", err
		}
		partes[i] = strconv.FormatInt(n, 10)
	}
	return "[" + strings.Join(partes, ", ") + "]", nil
}

func inteiro(valor any) (int64, error) {
	switch v := valor.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		if v != math.Trunc(v) || v > math.MaxInt64 || v < math.MinInt64 {
			return 0, fmt.Errorf("valor precisa ser inteiro: %v", v)
		}
		return int64(v), nil
	}
	return 0, fmt.Errorf("valor precisa ser inteiro: %v", valor)
}

func textos(valor any) ([]string, error) {
	switch v := valor.(type) {
	case string:
		return []string{v}, nil
	case []any:
		if len(v) == 0 {
			break
		}
		saida := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("sinal precisa ser texto: %v", item)
			}
			saida[i] = s
		}
		return saida, nil
	}
	return nil, errors.New("fraude.sinais precisa de um texto ou lista de textos")
}
