package main

import (
	"github.com/labstack/echo/v4"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// maxRewriteBody ограничивает размер ответа, который читается в память для замены ссылок
const maxRewriteBody = 32 << 20

// hop-by-hop заголовки не должны проходить через прокси (RFC 9110, 7.6.1)
var hopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// заголовки, раскрывающие exhentai IP конечного пользователя
var forwardedHeaders = []string{
	"Forwarded",
	"X-Forwarded-For",
	"X-Forwarded-Host",
	"X-Forwarded-Proto",
	"X-Real-Ip",
}

// заголовки upstream (Cloudflare/exhentai), которые описывают их хост, а не прокси:
// Alt-Svc отправляет браузер на HTTP/3 к прокси, HSTS с preload закрепляет домен за https
var upstreamOnlyHeaders = []string{
	"Alt-Svc",
	"Cf-Cache-Status",
	"Cf-Ray",
	"Nel",
	"Report-To",
	"Server",
	"Strict-Transport-Security",
	"Via",
}

// типы ответов, в которых переписываются ссылки на exhentai
var rewriteTypes = []string{
	"application/javascript",
	"application/x-javascript",
	"application/json",
	"application/xml",
	"application/xhtml+xml",
}

var client *http.Client

func initClient() {
	dialer := &net.Dialer{
		Timeout:   conf.connTimeout,
		KeepAlive: conf.connKeepAlive,
	}
	client = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: conf.respTimeout,
			MaxIdleConnsPerHost:   conf.connMaxIdle,
			DisableCompression:    true,
		},
	}
}

func removeHopHeaders(h http.Header) {
	for _, f := range h.Values("Connection") {
		for _, k := range strings.Split(f, ",") {
			if k = strings.TrimSpace(k); k != "" {
				h.Del(k)
			}
		}
	}
	for _, k := range hopHeaders {
		h.Del(k)
	}
}

func shouldRewrite(contentType string) bool {
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	for _, t := range rewriteTypes {
		if contentType == t {
			return true
		}
	}
	return false
}

// thumbPrefix — путь, через который проксируется хост обложек s.exhentai.org,
// чтобы не заводить под него отдельный поддомен с DNS и сертификатом
const thumbPrefix = "/__s"

func rewrite(s string) string {
	// s.exhentai.org заменяется раньше exhentai.org, иначе превратится в несуществующий s.<rootHost>
	s = strings.ReplaceAll(s, "https://s.exhentai.org", conf.rootPath+thumbPrefix)
	s = strings.ReplaceAll(s, "//s.exhentai.org", conf.rootPath+thumbPrefix)
	s = strings.ReplaceAll(s, "https://exhentai.org", conf.rootPath)
	// без PANDA_ROOT_HOST голый хост не трогаем, иначе он просто вырезается из текста
	if conf.rootHost != "" {
		s = strings.ReplaceAll(s, "exhentai.org", conf.rootHost)
	}
	return s
}

func handle(c echo.Context) error {
	pc := c.(*PandaContext)
	req := c.Request()

	// parse request
	upstream, uri := "https://exhentai.org", req.URL.RequestURI()
	if strings.HasPrefix(uri, thumbPrefix+"/") {
		upstream, uri = "https://s.exhentai.org", strings.TrimPrefix(uri, thumbPrefix)
	}
	proxyReq, err := http.NewRequestWithContext(req.Context(), req.Method, upstream+uri, req.Body)
	if err != nil {
		return c.String(http.StatusBadRequest, "bad request")
	}

	// fix header
	proxyReq.Header = req.Header.Clone()
	removeHopHeaders(proxyReq.Header)
	for _, k := range forwardedHeaders {
		proxyReq.Header.Del(k)
	}
	proxyReq.Header.Del("Authorization")
	proxyReq.Header.Del("Cookie")
	proxyReq.Header.Del("Accept-Encoding")
	proxyReq.Header.Del("Referer")
	if proxyReq.Header.Get("Origin") != "" {
		proxyReq.Header.Set("Origin", "https://exhentai.org")
	}
	for _, c := range req.Cookies() {
		if c.Name == "ipb_member_id" || c.Name == "ipb_pass_hash" {
			continue
		}
		proxyReq.AddCookie(c)
	}
	proxyReq.AddCookie(&http.Cookie{Name: "ipb_member_id", Value: pc.MemberID})
	proxyReq.AddCookie(&http.Cookie{Name: "ipb_pass_hash", Value: pc.PassHash})
	proxyReq.ContentLength = req.ContentLength

	// send request
	resp, err := client.Do(proxyReq)
	if err != nil {
		return c.String(http.StatusBadGateway, "upstream error")
	}
	defer resp.Body.Close()

	// copy headers, keeping repeated values
	removeHopHeaders(resp.Header)
	for _, k := range upstreamOnlyHeaders {
		resp.Header.Del(k)
	}
	h := c.Response().Header()
	for k, vs := range resp.Header {
		switch k {
		case "Content-Length", "Set-Cookie", "Location":
			continue
		}
		for _, v := range vs {
			h.Add(k, v)
		}
	}
	for _, sc := range resp.Header.Values("Set-Cookie") {
		h.Add("Set-Cookie", strings.ReplaceAll(sc, "; domain=.exhentai.org", ""))
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		h.Set("Location", rewrite(loc))
	}

	// check content type
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !shouldRewrite(strings.ToLower(contentType)) {
		return c.Stream(resp.StatusCode, contentType, resp.Body)
	}

	// replace content
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRewriteBody+1))
	if err != nil {
		return c.String(http.StatusBadGateway, "upstream error")
	}
	if len(body) > maxRewriteBody {
		return c.String(http.StatusBadGateway, "upstream response too large")
	}

	return c.Stream(resp.StatusCode, contentType, strings.NewReader(rewrite(string(body))))
}
