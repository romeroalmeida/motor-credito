package httpapi

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
)

// Mudanca é o schema Mudanca do OpenAPI.
type Mudanca struct {
	Caminho string `json:"caminho"`
	Tipo    string `json:"tipo"` // adicionado | removido | alterado
	Antes   any    `json:"antes,omitempty"`
	Depois  any    `json:"depois,omitempty"`
}

// diferencas compara dois valores pelo JSON deles e lista o que mudou, com caminhos como "faixas[1].taxa_bps".
func diferencas(antes, depois any) ([]Mudanca, error) {
	a, err := comoJSON(antes)
	if err != nil {
		return nil, err
	}
	d, err := comoJSON(depois)
	if err != nil {
		return nil, err
	}
	mudancas := []Mudanca{}
	comparar("", a, d, &mudancas)
	return mudancas, nil
}

func comoJSON(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var saida any
	return saida, json.Unmarshal(b, &saida)
}

func comparar(caminho string, a, d any, saida *[]Mudanca) {
	switch va := a.(type) {
	case map[string]any:
		if vd, ok := d.(map[string]any); ok {
			chaves := map[string]bool{}
			for k := range va {
				chaves[k] = true
			}
			for k := range vd {
				chaves[k] = true
			}
			ordem := make([]string, 0, len(chaves))
			for k := range chaves {
				ordem = append(ordem, k)
			}
			sort.Strings(ordem)
			for _, k := range ordem {
				filho := k
				if caminho != "" {
					filho = caminho + "." + k
				}
				compararFilho(filho, va, vd, k, saida)
			}
			return
		}
	case []any:
		if vd, ok := d.([]any); ok {
			for i := 0; i < max(len(va), len(vd)); i++ {
				filho := caminho + "[" + strconv.Itoa(i) + "]"
				switch {
				case i >= len(vd):
					*saida = append(*saida, Mudanca{Caminho: filho, Tipo: "removido", Antes: va[i]})
				case i >= len(va):
					*saida = append(*saida, Mudanca{Caminho: filho, Tipo: "adicionado", Depois: vd[i]})
				default:
					comparar(filho, va[i], vd[i], saida)
				}
			}
			return
		}
	}
	if !reflect.DeepEqual(a, d) {
		*saida = append(*saida, Mudanca{Caminho: caminho, Tipo: "alterado", Antes: a, Depois: d})
	}
}

func compararFilho(caminho string, a, d map[string]any, k string, saida *[]Mudanca) {
	va, temA := a[k]
	vd, temD := d[k]
	switch {
	case !temD:
		*saida = append(*saida, Mudanca{Caminho: caminho, Tipo: "removido", Antes: va})
	case !temA:
		*saida = append(*saida, Mudanca{Caminho: caminho, Tipo: "adicionado", Depois: vd})
	default:
		comparar(caminho, va, vd, saida)
	}
}
