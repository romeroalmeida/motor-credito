package painelauth

import (
	"crypto/subtle"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
)

const (
	emissorTOTP = "Motor de Crédito"
	periodoTOTP = 30
)

var opcoesTOTP = totp.ValidateOpts{Period: periodoTOTP, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}

// CifrarTOTP guarda o segredo com subchave própria (derivada da mestra) e AAD = usuario_id: a linha copiada para
// outro usuário não decifra. A semente usa a mesma função.
func CifrarTOTP(chaveMestra []byte, usuarioID, segredo string) ([]byte, error) {
	return plataforma.CifrarAAD(plataforma.DerivarChave(chaveMestra, "totp"), []byte(segredo), []byte(usuarioID))
}

// DecifrarTOTP desfaz CifrarTOTP.
func DecifrarTOTP(chaveMestra []byte, usuarioID string, cifrado []byte) (string, error) {
	claro, err := plataforma.DecifrarAAD(plataforma.DerivarChave(chaveMestra, "totp"), cifrado, []byte(usuarioID))
	return string(claro), err
}

// novoSegredoTOTP gera o segredo base32 e a URL otpauth:// para o QR code.
func novoSegredoTOTP(email string) (segredo, url string, err error) {
	k, err := totp.Generate(totp.GenerateOpts{Issuer: emissorTOTP, AccountName: email})
	if err != nil {
		return "", "", err
	}
	return k.Secret(), k.URL(), nil
}

// passoDoCodigo devolve o passo TOTP (tempo/30 s) em que o código confere, aceitando um passo de folga
// para cada lado. O passo é gravado em totp.ultimo_passo para o mesmo código não valer duas vezes.
func passoDoCodigo(segredo, codigo string, agora time.Time) (int64, bool) {
	if len(codigo) != 6 {
		return 0, false
	}
	atual := agora.Unix() / periodoTOTP
	for _, folga := range []int64{-1, 0, 1} {
		passo := atual + folga
		esperado, err := totp.GenerateCodeCustom(segredo, time.Unix(passo*periodoTOTP, 0), opcoesTOTP)
		if err == nil && subtle.ConstantTimeCompare([]byte(esperado), []byte(codigo)) == 1 {
			return passo, true
		}
	}
	return 0, false
}
