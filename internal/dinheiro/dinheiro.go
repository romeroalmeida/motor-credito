// Package dinheiro guarda as contas de dinheiro: sempre centavos em int64, taxa em pontos-base.
package dinheiro

import "math/big"

// Centavos é o único tipo de valor monetário guardado ou devolvido.
type Centavos = int64

// Parcela calcula a prestação pela tabela Price: PMT = V·i·(1+i)^n / ((1+i)^n − 1), com i = taxaBps/10000 ao mês.
// Taxa 0 → valor/prazo. Conta em big.Rat (exata) e arredonda meio-para-cima no centavo.
// Prazo, valor ou taxa não positivos (taxa negativa) devolvem 0: não há parcela que faça sentido.
func Parcela(valor Centavos, prazoMeses int, taxaBps int) Centavos {
	if valor <= 0 || prazoMeses <= 0 || taxaBps < 0 {
		return 0
	}
	v := new(big.Rat).SetInt64(valor)
	if taxaBps == 0 {
		return arredondar(v.Quo(v, big.NewRat(int64(prazoMeses), 1)))
	}
	i := big.NewRat(int64(taxaBps), 10000)
	fator := potencia(new(big.Rat).Add(big.NewRat(1, 1), i), prazoMeses) // (1+i)^n
	num := new(big.Rat).Mul(v, i)
	num.Mul(num, fator)
	den := new(big.Rat).Sub(fator, big.NewRat(1, 1))
	return arredondar(num.Quo(num, den))
}

func potencia(base *big.Rat, n int) *big.Rat {
	num := new(big.Int).Exp(base.Num(), big.NewInt(int64(n)), nil)
	den := new(big.Int).Exp(base.Denom(), big.NewInt(int64(n)), nil)
	return new(big.Rat).SetFrac(num, den)
}

// arredondar faz meio-para-cima para valores não negativos: piso(x + 1/2).
func arredondar(x *big.Rat) Centavos {
	x = new(big.Rat).Add(x, big.NewRat(1, 2))
	q := new(big.Int).Quo(x.Num(), x.Denom()) // trunca; x ≥ 0, então é o piso
	return q.Int64()
}
