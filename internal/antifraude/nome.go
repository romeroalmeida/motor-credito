package antifraude

import (
	"strings"
	"unicode"
)

// semAcento cobre os acentos do português; golang.org/x/text não é dependência direta do módulo.
var semAcento = strings.NewReplacer(
	"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
	"É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N",
)

var preposicoes = map[string]bool{"DA": true, "DE": true, "DO": true, "DAS": true, "DOS": true, "E": true, "D": true}

// tokensNome devolve o nome em maiúsculas, sem acento, sem pontuação e sem preposições.
func tokensNome(nome string) []string {
	limpo := semAcento.Replace(strings.ToUpper(nome))
	campos := strings.FieldsFunc(limpo, func(r rune) bool { return !unicode.IsLetter(r) })
	tokens := campos[:0]
	for _, t := range campos {
		if !preposicoes[t] {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// sobrenomes são os tokens depois do primeiro nome; nome de um token só vale como o próprio sobrenome.
func sobrenomes(nome string) []string {
	t := tokensNome(nome)
	if len(t) > 1 {
		return t[1:]
	}
	return t
}

// nomeDivergente: nenhum sobrenome informado aparece entre os sobrenomes do bureau.
func nomeDivergente(informado, doBureau string) bool {
	doBureauSet := map[string]bool{}
	for _, s := range sobrenomes(doBureau) {
		doBureauSet[s] = true
	}
	for _, s := range sobrenomes(informado) {
		if doBureauSet[s] {
			return false
		}
	}
	return true
}
