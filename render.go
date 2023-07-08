package hg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/worldiety/hg/form"
	"golang.org/x/exp/slog"
	"html/template"
	"mime"
	"net/http"
	"net/http/httptest"
	"reflect"
)

// ViewHtmlFunc transforms a Model into a raw HTML string.
type ViewHtmlFunc[Model any] func(model Model) template.HTML

func (f ViewHtmlFunc[Model]) Handle(model Model) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(f(model)))
	}
}

// UpdateFunc mutates the model by applying the Msg and returning an altered Model.
type UpdateFunc[Model, Msg any] func(model Model, msg Msg) Model

// RequestUpdateFunc mutates the model by decode and applying the msg and returning an altered Model.
type RequestUpdateFunc[Model any] func(model Model, r *http.Request) (Model, error)

// RenderOptions provides a package private Options pattern.
type RenderOption[Model any] func(hnd *rHnd[Model])

type rHnd[Model any] struct {
	renderer  ViewFunc[Model]
	decoders  map[string]MsgHandler[Model]
	onRequest UpdReqFunc[Model]
	maxMemory int64
}

type jsRedirectModel struct {
	TargetURL string `json:"target"`
	NavDir    string `json:"navDir"` // forward|backward|replace
	State     any    `json:"state"`  // anything
	MsgType   string `json:"msgType"`
	MsgData   string `json:"msgData"`
}

func (h rHnd[Model]) handle(writer http.ResponseWriter, request *http.Request) {
	var state Model

	// a simple get request just passes the empty default model through the rendering
	if request.Method == http.MethodGet {
		// if defined, pre-process state
		if h.onRequest != nil {
			state = h.onRequest(request, state)
		}

		h.renderer(writer, request, state)
		return
	}

	// usually a POST and only form-Data allowed for state and event submission (and/or an actual form)
	mtype, _, _ := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if mtype != "multipart/form-Data" {
		slog.Error("hg expected form-Data", slog.String("Content-Type", mtype))
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := request.ParseMultipartForm(h.maxMemory); err != nil {
		slog.Error("failed to parse multipart form", slog.Any("err", err))
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// check the event type
	evtType := request.PostFormValue("_eventType")
	var hnd MsgHandler[Model]
	if evtType != "!refresh" {
		dec, ok := h.decoders[evtType]
		if !ok {
			slog.Error("unknown message type", slog.String("type", evtType))
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		hnd = dec
	}

	// we have always the state in the form, load it
	stateText := request.PostFormValue("_state")
	dec := json.NewDecoder(bytes.NewReader([]byte(stateText)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&state); err != nil {
		slog.Error("cannot unmarshal state from form: %w", slog.Any("err", err))
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// if defined, pre-process state
	if h.onRequest != nil {
		state = h.onRequest(request, state)
	}

	// invoke the handler and process event Data / payload
	if hnd != nil {
		// hnd is nil, if we got a !refresh
		s, err := hnd.Transform(state, request)
		if err != nil {
			slog.Error("failed to transform msg", slog.String("url", request.URL.String()), slog.Any("err", err))
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		state = s
	}

	if redirect, ok := any(state).(redirectable); ok && redirect.Redirection().redirect {
		writer.Header().Set("Content-Type", "application/json")
		rd := redirect.Redirection()
		buf, err := json.Marshal(jsRedirectModel{
			TargetURL: rd.url,
			NavDir:    rd.direction,
			State:     nil, //TODO
			MsgType:   "",  //TODO
			MsgData:   "",  //TODO
		})

		if err != nil {
			slog.Error("failed to encode redirection", slog.Any("err", err))
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		if _, err := writer.Write(buf); err != nil {
			slog.Error("failed to write http response", slog.Any("err", err))
		}

		return
	}

	h.renderer(writer, request, state)
}

type ViewFunc[Model any] func(w http.ResponseWriter, r *http.Request, model Model)
type UpdReqFunc[Model any] func(r *http.Request, model Model) Model

func (f ViewFunc[Model]) Handle(model Model) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		f(writer, request, model)
	}
}

// ToString applies an empty request to invoke the ViewFunc and converts the result without validation
// into a string.
// This is mainly for debugging purposes.
func (f ViewFunc[Model]) ToString(model Model) string {
	w := httptest.NewRecorder()
	f(w, &http.Request{}, model)
	return w.Body.String()
}

// A MsgHandler transforms an incoming request into a new Model state.
type MsgHandler[Model any] interface {
	Alias() string

	Transform(in Model, r *http.Request) (Model, error)
}

// only works for unique shape types
type defaultMsgDecoder[M any] struct {
	maxMemory   int64
	alias       string
	onTransform func(model M, r *http.Request) (M, error)
}

func (g *defaultMsgDecoder[M]) Transform(in M, r *http.Request) (M, error) {
	return g.onTransform(in, r)
}

func (g *defaultMsgDecoder[M]) Alias() string {
	return g.alias

}

// Case is invoked if the message alias is matched and tries to unmarshal the form value _eventData message into a new value
// of type Msg. It then calls the UpdateFunc to transform the given Model into a new state.
// To apply navigation, see also [Redirect].
func Case[Model, Msg any](alias string, update UpdateFunc[Model, Msg]) MsgHandler[Model] {
	decoder := &defaultMsgDecoder[Model]{
		alias:     alias,
		maxMemory: 10 * 1024 * 1024,
	}

	decoder.onTransform = func(model Model, r *http.Request) (Model, error) {
		var msg Msg

		// either we have eventData, or we assume a full form (or nothing)
		if eventDataText := r.PostFormValue("_eventData"); eventDataText != "" {
			if err := json.Unmarshal([]byte(eventDataText), &msg); err != nil {
				return model, fmt.Errorf("cannot unmarshal event Data form field: %w", err)
			}
		} else {
			if err := form.Unmarshal(&msg, r); err != nil {
				return model, fmt.Errorf("cannot unmarshal form into message: %w", err)
			}
		}

		return update(model, msg), nil
	}

	return decoder
}

// CaseWithQualifier is like [Case] but uses as alias the package path and the type Name of the Message.
func CaseWithQualifier[Model, Msg any](update UpdateFunc[Model, Msg]) MsgHandler[Model] {
	var msg Msg
	t := reflect.TypeOf(msg)
	alias := t.PkgPath() + "." + t.Name()

	return Case[Model, Msg](alias, update)
}

// OnRequest sets the first untyped transformer which is called for each request to the handler.
// This always happens, independently if a message has been sent.
// Thus, this transformer is especially helpful to inject any transitional state, which cannot be
// serialized, even things like contexts or use case services.
func OnRequest[Model any](f UpdReqFunc[Model]) RenderOption[Model] {
	return func(hnd *rHnd[Model]) {
		hnd.onRequest = f
	}
}

// Update is the entry point to register a bunch of message handlers.
// See also [Case].
func Update[Model any](messages ...MsgHandler[Model]) RenderOption[Model] {
	return func(hnd *rHnd[Model]) {
		for _, msgDecoder := range messages {
			hnd.decoders[msgDecoder.Alias()] = msgDecoder
			fmt.Println("=>", msgDecoder.Alias())
		}
	}
}

// Handler builds a http handlerfunc based on a model-view transformation
func Handler[Model any](renderer ViewFunc[Model], options ...RenderOption[Model]) http.HandlerFunc {
	hnd := &rHnd[Model]{
		renderer: func(w http.ResponseWriter, r *http.Request, model Model) {
			w.WriteHeader(http.StatusNotImplemented)
			buf, err := json.Marshal(model)
			if err != nil {
				buf = []byte(err.Error())
			}

			_, _ = w.Write(buf)
		},
		decoders: map[string]MsgHandler[Model]{},
	}

	if renderer != nil {
		hnd.renderer = renderer
	}

	for _, option := range options {
		option(hnd)
	}

	return hnd.handle
}
