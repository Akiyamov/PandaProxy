package main

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	loadConfig()
	initClient()

	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(ApplyPandaContext)
	e.Use(RetrieveCredentials)

	e.Any("/*", handle)

	e.Server.ReadHeaderTimeout = 10 * time.Second
	e.Server.IdleTimeout = 120 * time.Second

	e.Logger.Fatal(e.Start(":60394"))
}
