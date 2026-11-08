package antifraude

import (
	"context"
	_ "embed"
	"net/netip"
	"strings"
)

var (
	//go:embed listas/ip_brasil.txt
	listaIPBrasil string
	//go:embed listas/ip_datacenter.txt
	listaIPDatacenter string
	//go:embed listas/email_descartavel.txt
	listaEmailDescartavel string

	faixasBrasil       = mustPrefixos(listaIPBrasil)
	faixasDatacenter   = mustPrefixos(listaIPDatacenter)
	emailsDescartaveis = conjunto(listaEmailDescartavel)
)

// linhas ignora vazias e comentários (#).
func linhas(texto string) []string {
	var out []string
	for _, l := range strings.Split(texto, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}

// mustPrefixos entra em pânico na carga do pacote: lista embutida quebrada é erro de build, não de execução.
func mustPrefixos(texto string) []netip.Prefix {
	var out []netip.Prefix
	for _, l := range linhas(texto) {
		out = append(out, netip.MustParsePrefix(l))
	}
	return out
}

func conjunto(texto string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, l := range linhas(texto) {
		out[strings.ToLower(l)] = struct{}{}
	}
	return out
}

func contem(faixas []netip.Prefix, ip netip.Addr) bool {
	for _, f := range faixas {
		if f.Contains(ip) {
			return true
		}
	}
	return false
}

// checarIP: IP inválido ou não roteável (privado, loopback) não dá para geolocalizar e conta como pulado.
func checarIP(_ context.Context, e Entrada, _ Contador) (bool, []string, error) {
	ip, err := netip.ParseAddr(strings.TrimSpace(e.IP))
	if err != nil {
		return false, nil, nil
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false, nil, nil
	}
	var sinais []string
	if !contem(faixasBrasil, ip) {
		sinais = append(sinais, SinalIPEstrangeiro)
	}
	if contem(faixasDatacenter, ip) {
		sinais = append(sinais, SinalIPDatacenter)
	}
	return true, sinais, nil
}
