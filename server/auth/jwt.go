package auth

import (
    "time"
    "github.com/dgrijalva/jwt-go"
    "github.com/golang/glog"

    "garydmenezes.com/mathgame/server/common"
)

c, err := common.ReadConfig("conf.json")
if err != nil {
	glog.Fatal(err)
}

func GenerateToken(uid uint) (string, error) {
	claims := jwt.MapClaims{}
	claims["user_id"] = uid
	claims["exp"] = time.Now().Add(time.Hour * c.AuthExp).Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(c.AuthSecretKey)
}

func VerifyToken(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHS256); !ok {
			return nil, fmt.Errorf("Unexpected signing method")
		}
		return c.AuthSecretKey, nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("Invalid token")
}
