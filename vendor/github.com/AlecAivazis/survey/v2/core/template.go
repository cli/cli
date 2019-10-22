package core

import (
	"bytes"
	"sync"
	"text/template"

	"github.com/mgutz/ansi"
)

// DisableColor can be used to make testing reliable
var DisableColor = false

var TemplateFuncs = map[string]interface{}{
	// Templates with Color formatting. See Documentation: https://github.com/mgutz/ansi#style-format
	"color": func(color string) string {
		if DisableColor {
			return ""
		}
		return ansi.ColorCode(color)
	},
}

func RunTemplate(tmpl string, data interface{}) (string, error) {
	t, err := getTemplate(tmpl)
	if err != nil {
		return "", err
	}
	buf := bytes.NewBufferString("")
	err = t.Execute(buf, data)
	if err != nil {
		return "", err
	}
	return buf.String(), err
}

var (
	memoizedGetTemplate = map[string]*template.Template{}

	memoMutex = &sync.RWMutex{}
)

func getTemplate(tmpl string) (*template.Template, error) {
	memoMutex.RLock()
	if t, ok := memoizedGetTemplate[tmpl]; ok {
		memoMutex.RUnlock()
		return t, nil
	}
	memoMutex.RUnlock()

	t, err := template.New("prompt").Funcs(TemplateFuncs).Parse(tmpl)
	if err != nil {
		return nil, err
	}

	memoMutex.Lock()
	memoizedGetTemplate[tmpl] = t
	memoMutex.Unlock()
	return t, nil
}
