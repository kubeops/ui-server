/*
Copyright AppsCode Inc. and Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package shared

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/pkg/errors"
)

func RenderTemplate(text string, data interface{}, buf *bytes.Buffer) (string, error) {
	if !strings.Contains(text, "{{") {
		return text, nil
	}

	tpl, err := template.New("").Funcs(sprig.TxtFuncMap()).Parse(text)
	if err != nil {
		return "", errors.Wrapf(err, "falied to parse template %s", text)
	}
	// Do nothing and continue execution.
	// If printed, the result of the index operation is the string "<no value>".
	// We mitigate that later.
	tpl.Option("missingkey=default")
	buf.Reset()
	err = tpl.Execute(buf, data)
	if err != nil {
		return "", errors.Wrapf(err, "falied to render template %s", text)
	}
	return strings.ReplaceAll(buf.String(), "<no value>", ""), nil
}
