package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rawWireTestConfig struct {
	src string
}

func (c rawWireTestConfig) OutputDirectory() string  { return filepath.Join(c.src, ".output") }

func (c rawWireTestConfig) OutputBaseName() string   { return "output" }

func (c rawWireTestConfig) SourceDirectory() string  { return c.src }

func (c rawWireTestConfig) SCSSCommandValue() string { return "" }

func (c rawWireTestConfig) SCSSDisabledValue() bool  { return true }

func TestWireRawHandlerEmitsTypedWrappers(t *testing.T) {
	dir := t.TempDir()

	c := New()
	c.SetBuildConfig("build_config", rawWireTestConfig{src: dir})

	c.AddSource("std_http.sova", stdHttpStubSrc)
	c.AddSource("app.sova", `package app on backend

import "std/http"

wire(transport: "raw", method: "GET", path: "/echo")
func echo(req: http.Request, res: http.Response) {
    let name = http.query(req, "name")
    http.setCookie(res, "last_name", name, 60, true, false, "Lax", "/")
    http.setStatus(res, 200)
    http.writeText(res, "hi " + name)
}
`)

	if err := c.Compile(); err != nil {
		t.Fatalf("compile: %v", err)
	}

	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}

	out, err := os.ReadFile(filepath.Join(dir, ".output", "output.go"))
	if err != nil {
		t.Fatalf("read emitted go: %v", err)
	}

	goSrc := string(out)

	for _, want := range []string{
		"__wireHandler",
		"http.ResponseWriter",
		"*http.Request",
		"Raw: r",
		"Raw: w",
	} {
		if !strings.Contains(goSrc, want) {
			t.Fatalf("emitted Go missing %q\n--- output.go ---\n%s", want, goSrc)
		}
	}
}

func TestWireRawHandlerMethodCallStyle(t *testing.T) {
	dir := t.TempDir()

	c := New()
	c.SetBuildConfig("build_config", rawWireTestConfig{src: dir})

	c.AddSource("std_http.sova", stdHttpStubSrc)
	c.AddSource("app.sova", `package app on backend

import "std/http"

wire(transport: "raw", method: "GET", path: "/echo2")
func echo2(req: http.Request, res: http.Response) {
    let name = req.query("name")
    res.setStatus(200)
    res.writeText("hi " + name)
}
`)

	if err := c.Compile(); err != nil {
		t.Fatalf("compile: %v", err)
	}

	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("method-form compile produced errors")
	}
}

func TestWireRawRejectsBareAnyParams(t *testing.T) {
	dir := t.TempDir()

	c := New()
	c.SetBuildConfig("build_config", rawWireTestConfig{src: dir})

	c.AddSource("std_http.sova", stdHttpStubSrc)
	c.AddSource("app.sova", `package app on backend

import "std/http"

wire(transport: "raw", method: "GET", path: "/echo")
func echo(req: any, res: any) {
}
`)

	_ = c.Check()
	if !c.Diag.Errored() {
		t.Fatalf("expected diagnostic for bare any params, got none")
	}

	found := false
	for _, d := range c.Diag.Diagnostics() {
		if strings.Contains(d.Msg, "http.Request") && strings.Contains(d.Msg, "http.Response") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("missing typed-param diagnostic: %+v", c.Diag.Diagnostics())
	}
}

const stdHttpStubSrc = `package std/http on backend

type Request {
    raw: any = none

    func query(name: string): string {
        return __reqQuery(this.raw, name)
    }
}

type Response {
    raw: any = none

    func setStatus(code: int) {
        __resSetStatus(this.raw, code)
    }

    func writeText(body: string) {
        __resWriteText(this.raw, body)
    }
}

extern {
    func __reqQuery(rawReq: any, name: string): string = {
        backend: "func(req any, n string) string { return req.(*http.Request).URL.Query().Get(n) }"
    }

    func __resSetStatus(rawRes: any, code: int) = {
        backend: "func(res any, code int) { res.(http.ResponseWriter).WriteHeader(code) }"
    }

    func __resSetCookie(rawRes: any, name: string, value: string, maxAgeSeconds: int, httpOnly: bool, secure: bool, sameSite: string, cookiePath: string) = {
        backend: "func(res any, name string, value string, maxAge int, httpOnly bool, secure bool, sameSite string, cookiePath string) { http.SetCookie(res.(http.ResponseWriter), &http.Cookie{Name: name, Value: value, MaxAge: maxAge, HttpOnly: httpOnly, Secure: secure, Path: cookiePath}) }"
    }

    func __resWriteText(rawRes: any, body: string) = {
        backend: "func(res any, body string) { _, _ = res.(http.ResponseWriter).Write([]byte(body)) }"
    }
}

func query(req: Request, name: string): string {
    return __reqQuery(req.raw, name)
}

func setStatus(res: Response, code: int) {
    __resSetStatus(res.raw, code)
}

func setCookie(res: Response, name: string, value: string, maxAgeSeconds: int, httpOnly: bool, secure: bool, sameSite: string, cookiePath: string) {
    __resSetCookie(res.raw, name, value, maxAgeSeconds, httpOnly, secure, sameSite, cookiePath)
}

func writeText(res: Response, body: string) {
    __resWriteText(res.raw, body)
}
`
