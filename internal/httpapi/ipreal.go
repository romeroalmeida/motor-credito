package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// IPReal reescreve r.RemoteAddr com o IP do cliente, lido do X-Forwarded-For, só quando a conexão vem de um proxy
// confiável. O cabeçalho é lido da direita para a esquerda pulando os proxies confiáveis: o primeiro endereço não
// confiável é o cliente (o que vem antes dele o próprio cliente pode ter forjado). Lista vazia = não confia em
// ninguém. Roda antes de tudo que usa o IP (limitadores, lista de IPs da chave, sessão).
func IPReal(confiaveis []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip, ok := ipDoCliente(r, confiaveis); ok {
				r.RemoteAddr = netip.AddrPortFrom(ip, 0).String()
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ipDoCliente(r *http.Request, confiaveis []netip.Prefix) (netip.Addr, bool) {
	par, ok := endereco(r.RemoteAddr)
	if !ok || !contem(confiaveis, par) {
		return netip.Addr{}, false
	}
	saltos := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	var cliente netip.Addr
	for i := len(saltos) - 1; i >= 0; i-- {
		ip, ok := endereco(strings.TrimSpace(saltos[i]))
		if !ok {
			return netip.Addr{}, false // cabeçalho ilegível: fica o endereço da conexão
		}
		cliente = ip
		if !contem(confiaveis, ip) {
			break
		}
	}
	return cliente, cliente.IsValid()
}

// endereco aceita "ip" ou "ip:porta" ("[v6]:porta").
func endereco(v string) (netip.Addr, bool) {
	if host, _, err := net.SplitHostPort(v); err == nil {
		v = host
	}
	ip, err := netip.ParseAddr(v)
	return ip.Unmap(), err == nil
}

func contem(prefixos []netip.Prefix, ip netip.Addr) bool {
	for _, p := range prefixos {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
