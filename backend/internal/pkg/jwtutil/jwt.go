// Package jwtutil 封装管理员 JWT（HS256）的签发与校验。
// Claims 结构与有效期约定见 docs/api-contract.md 第 0.3 节。
package jwtutil

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer 是固定的签发者标识。
const Issuer = "liltidc-oem"

// Claims 是管理员 token 的载荷。
type Claims struct {
	AdminID  uint   `json:"admin_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// 校验失败的语义化错误（中间件据此返回 401）。
var (
	ErrInvalid = errors.New("token 无效")
	ErrExpired = errors.New("token 已过期")
)

// Sign 签发 token，返回 token 字符串与到期时间。
func Sign(secret string, expire time.Duration, adminID uint, username string) (string, time.Time, error) {
	if secret == "" {
		return "", time.Time{}, errors.New("JWT secret 为空，无法签发 token")
	}
	now := time.Now()
	expiresAt := now.Add(expire)
	claims := Claims{
		AdminID:  adminID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("签发 token 失败: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse 校验并解析 token。签名算法固定为 HMAC 家族，防算法混淆攻击。
func Parse(secret, tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("非预期的签名算法: %v", t.Header["alg"])
			}
			return []byte(secret), nil
		},
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpired
		}
		return nil, ErrInvalid
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalid
	}
	return claims, nil
}
