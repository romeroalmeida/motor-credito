//go:build ignore

// Imprime o código TOTP atual de um segredo base32 (o que a semente mostra), para o Postman/newman:
//
//	go run scripts/totp.go JBSWY3DPEHPK3PXP
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pquerna/otp/totp"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "uso: go run scripts/totp.go <segredo base32>")
		os.Exit(2)
	}
	codigo, err := totp.GenerateCode(os.Args[1], time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "segredo inválido:", err)
		os.Exit(1)
	}
	fmt.Println(codigo)
}
