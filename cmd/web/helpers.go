package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/go-playground/form/v4"
	"net/http"
	"runtime/debug"
)

func (a *application) serverError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())
	a.errorLog.Output(2, trace)

	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (a *application) clientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

func (a *application) notFound(w http.ResponseWriter) {
	a.clientError(w, http.StatusNotFound)
}

func (a *application) render(w http.ResponseWriter, status int, page string, data *templateData) {

	ts, ok := a.templateCache[page]

	if !ok {
		err := fmt.Errorf("the template %s does not exist", page)
		a.serverError(w, err)
		return
	}

	buf := new(bytes.Buffer)

	err := ts.ExecuteTemplate(buf, "base", data)

	if err != nil {
		a.serverError(w, err)
		return
	}
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (a *application) decodePostForm(r *http.Request, dst any) error {

	err := r.ParseForm()
	if err != nil {
		return err
	}

	err = a.formDecoder.Decode(dst, r.PostForm)
	if err != nil {

		var invalidDecoderError *form.InvalidDecoderError

		if errors.As(err, &invalidDecoderError) {
			panic(err)
		}
		return err
	}

	return nil
}
