package flyjwt

import (
	"errors"
	"fmt"
	"time"

	baseJwt "github.com/golang-jwt/jwt/v5"
)

type JwtUserClaims struct {
	ID    string `json:"id"`
	Role  string `json:"role"`
	Email string `json:"email"`
}

type JWTConfig struct {
	Secret     string
	Expiration time.Duration
}

func GenerateToken(model JwtUserClaims, cfg JWTConfig) (string, error) {
	claims := baseJwt.MapClaims{
		"sub":   model.ID,
		"email": model.Email,
		"role":  model.Role,
		"exp":   time.Now().Add(cfg.Expiration).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := baseJwt.NewWithClaims(baseJwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.Secret))
}

// ParseToken parses and validates a JWT string using HS256.
func ParseToken(tokenStr string, secret string) (baseJwt.MapClaims, error) {
	token, err := baseJwt.Parse(tokenStr, func(token *baseJwt.Token) (interface{}, error) {
		// Verify signature method is HMAC
		if _, ok := token.Method.(*baseJwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(baseJwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid jwt token")
}
