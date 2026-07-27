package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken agrupa cualquier motivo por el que un token no es
// válido (firma incorrecta, expirado, formato inesperado): quien llama
// no necesita distinguir el motivo exacto, solo que debe rechazar la
// petición con 401.
var ErrInvalidToken = errors.New("el token no es válido o expiró")

// IssueToken firma un nuevo JWT con el subject dado (el nombre de la
// credencial de servicio autenticada) y lo hace expirar tras ttl.
// Devuelve también el instante de expiración para incluirlo en la
// respuesta del login.
func IssueToken(secret, subject string, ttl time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().UTC().Add(ttl)

	claims := jwt.RegisteredClaims{
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// ValidateToken verifica la firma y expiración de tokenString y, si es
// válido, devuelve el subject original (quién se autenticó).
func ValidateToken(secret, tokenString string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return "", ErrInvalidToken
	}
	return claims.Subject, nil
}