// Package jwtutil 封装管理员 JWT（HS256）的签发与校验。
// Claims 结构与有效期约定见 docs/api-contract.md 第 0.3 节。
package jwtutil

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer 是管理员 token 的固定签发者标识。
const Issuer = "liltidc-oem"

// IssuerClient 是会员 token 的签发者标识（批 3-α）。
// 双签发方隔离：ParseClient 强制校验 issuer=liltidc-oem-client，
// 管理员 token 拿去访问会员接口（或反之）必然 401。
const IssuerClient = "liltidc-oem-client"

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

// ClientClaims 是会员 token 的载荷（批 3-α）。
type ClientClaims struct {
	ClientID uint   `json:"client_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// SignClient 签发会员 token（issuer=liltidc-oem-client）。
func SignClient(secret string, expire time.Duration, clientID uint, username string) (string, time.Time, error) {
	if secret == "" {
		return "", time.Time{}, errors.New("JWT secret 为空，无法签发 token")
	}
	now := time.Now()
	expiresAt := now.Add(expire)
	claims := ClientClaims{
		ClientID: clientID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    IssuerClient,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("签发会员 token 失败: %w", err)
	}
	return signed, expiresAt, nil
}

// ParseClient 校验并解析会员 token（强制 issuer=IssuerClient，管理员 token 必然失败）。
func ParseClient(secret, tokenString string) (*ClientClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &ClientClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("非预期的签名算法: %v", t.Header["alg"])
			}
			return []byte(secret), nil
		},
		jwt.WithIssuer(IssuerClient),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpired
		}
		return nil, ErrInvalid
	}
	claims, ok := token.Claims.(*ClientClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalid
	}
	return claims, nil
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
