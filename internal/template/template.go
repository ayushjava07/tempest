package template

import (
	"bytes"
	"fmt"
	"text/template"
)

type Engine struct {
	funcMap template.FuncMap
}

func NewEngine() *Engine {
	return &Engine{
		funcMap: template.FuncMap{
			"upper":   upper,
			"lower":   lower,
			"default": defaultVal,
		},
	}
}

func upper(s string) string {
	return template.HTMLEscapeString(s)
}

func lower(s string) string {
	return template.HTMLEscapeString(s)
}

func defaultVal(def, val string) string {
	if val == "" {
		return def
	}
	return val
}

func (e *Engine) AddFunc(name string, fn any) {
	if e.funcMap == nil {
		e.funcMap = template.FuncMap{}
	}
	e.funcMap[name] = fn
}

func (e *Engine) Parse(name, tmpl string) (*template.Template, error) {
	t := template.New(name).Funcs(e.funcMap)
	return t.Parse(tmpl)
}

func (e *Engine) Execute(name, tmpl string, data any) (string, error) {
	t, err := e.Parse(name, tmpl)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	return buf.String(), nil
}

func (e *Engine) MustExecute(name, tmpl string, data any) string {
	s, err := e.Execute(name, tmpl, data)
	if err != nil {
		panic(err)
	}
	return s
}

type Loader struct {
	engine *Engine
	tmpls  map[string]*template.Template
}

func NewLoader() *Loader {
	return &Loader{
		engine: NewEngine(),
		tmpls:  make(map[string]*template.Template),
	}
}

func (l *Loader) Load(name, content string) error {
	t, err := l.engine.Parse(name, content)
	if err != nil {
		return err
	}
	l.tmpls[name] = t
	return nil
}

func (l *Loader) Execute(name string, data any) (string, error) {
	t, ok := l.tmpls[name]
	if !ok {
		return "", fmt.Errorf("template not found: %s", name)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (l *Loader) Has(name string) bool {
	_, ok := l.tmpls[name]
	return ok
}