package utils

import (
	"crypto/rsa"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTManager struct {
	privateKey             *rsa.PrivateKey
	publicKey              *rsa.PublicKey
	accessTokenExpiration  time.Duration
	refreshTokenExpiration time.Duration
}

type JWTClaims struct {
	DeviceID    uint   `json:"device_id"`
	Fingerprint string `json:"fingerprint"`
	TokenType   string `json:"token_type"`
	jwt.RegisteredClaims
}

func NewJWTManager(privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey, accessExp, refreshExp time.Duration) *JWTManager {
	return &JWTManager{
		privateKey:             privateKey,
		publicKey:              publicKey,
		accessTokenExpiration:  accessExp,
		refreshTokenExpiration: refreshExp,
	}
}

func (j *JWTManager) GenerateAccessToken(deviceID uint, fingerprint string) (string, error) {
	claims := JWTClaims{
		DeviceID:    deviceID,
		Fingerprint: fingerprint,
		TokenType:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.accessTokenExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "brainnav-backend",
			Subject:   "device-auth",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(j.privateKey)
}

func (j *JWTManager) GenerateRefreshToken(deviceID uint, fingerprint string) (string, error) {
	claims := JWTClaims{
		DeviceID:    deviceID,
		Fingerprint: fingerprint,
		TokenType:   "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.refreshTokenExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "brainnav-backend",
			Subject:   "device-refresh",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(j.privateKey)
}

func (j *JWTManager) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
	return j.validateToken(tokenString, "access")
}

func (j *JWTManager) ValidateRefreshToken(tokenString string) (*JWTClaims, error) {
	return j.validateToken(tokenString, "refresh")
}

func (j *JWTManager) AccessTTL() time.Duration { return j.accessTokenExpiration }

func (j *JWTManager) RefreshTTL() time.Duration { return j.refreshTokenExpiration }

func (j *JWTManager) validateToken(tokenString, expectedType string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("invalid signing method")
		}
		return j.publicKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		if claims.TokenType != expectedType {
			return nil, errors.New("invalid token type")
		}
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
