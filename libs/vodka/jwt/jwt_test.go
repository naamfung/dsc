package jwt

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// jwtCustomClaims 是扩展的自定义声明
type jwtCustomClaims struct {
	jwt.RegisteredClaims        // 使用新的标准声明
	Name                 string `json:"name"`
	Admin                bool   `json:"admin"`
}

func TestJWT(t *testing.T) {
	// 创建一个新的 token
	claims := jwtCustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)), // 设置过期时间
		},
		Name:  "user_8",
		Admin: true,
	}

	// 将 claims 转换为 MapClaims
	claimsMap := jwt.MapClaims{
		"exp":   claims.ExpiresAt.Unix(),
		"name":  claims.Name,
		"admin": claims.Admin,
	}

	// 生成 token 字符串
	tokenString, err := NewTokenString("secret", AlgorithmHS256, claimsMap)
	assert.NoError(t, err)

	// 测试解析有效的JWT
	parsedToken, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return []byte("secret"), nil
	})
	assert.NoError(t, err)
	assert.True(t, parsedToken.Valid)

	// 测试解析无效的JWT
	invalidToken := tokenString + "invalid"
	_, err = jwt.Parse(invalidToken, func(t *jwt.Token) (interface{}, error) {
		return []byte("secret"), nil
	})
	assert.Error(t, err)

	// 测试过期的JWT
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)) // 设置过期时间为过去
	claimsMap = jwt.MapClaims{
		"exp":   claims.ExpiresAt.Unix(),
		"name":  claims.Name,
		"admin": claims.Admin,
	}

	expiredToken, err := NewTokenString("secret", AlgorithmHS256, claimsMap)
	assert.NoError(t, err)

	_, err = jwt.Parse(expiredToken, func(t *jwt.Token) (interface{}, error) {
		return []byte("secret"), nil
	})
	assert.Error(t, err) // 应该返回错误，因为token已过期
}
