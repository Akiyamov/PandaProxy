package main

import (
	"crypto/subtle"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"strconv"
)

type PandaContext struct {
	echo.Context
	MemberID string
	PassHash string
}

func ApplyPandaContext(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		pc := &PandaContext{Context: c}
		return next(pc)
	}
}

var RetrieveCredentials = middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
	pc := c.(*PandaContext)
	if subtle.ConstantTimeCompare([]byte(username), []byte(conf.publicUser)) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(conf.publicPassword)) == 1 {
		pc.MemberID = conf.publicMemberID
		pc.PassHash = conf.publicPassHash
	} else if _, err := strconv.Atoi(username); err == nil && len(password) == 32 {
		pc.MemberID = username
		pc.PassHash = password
	} else {
		return false, nil
	}
	return true, nil
})
