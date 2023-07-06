package form

import (
	"fmt"
	"github.com/worldiety/hg/internal"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
)

// Unmarshal takes the given request and parses the form into the given struct pointer.
func Unmarshal(dst any, src *http.Request) error {
	if reflect.ValueOf(dst).Kind() != reflect.Pointer {
		panic("dst must be a pointer")
	}

	typ := reflect.ValueOf(dst).Elem()

	for i := 0; i < typ.NumField(); i++ {
		val := typ.Field(i).Interface()
		if _, ok := val.(*multipart.Form); ok {
			typ.Field(i).Set(reflect.ValueOf(src.MultipartForm))
		}
	}

	for key, values := range src.MultipartForm.Value {
		if strings.HasPrefix(key, "_") {
			continue
		}

		field := typ.FieldByName(key)
		if !field.IsValid() {
			return fmt.Errorf("type %T does not have expected form field '%s'", dst, key)
		}

		if err := internal.ParseValue(field, values); err != nil {
			return fmt.Errorf("value %v cannot be parsed into field %T.%s: %w", values, dst, key, err)
		}
	}

	// parse blobs into a byte slice
	for key, headers := range src.MultipartForm.File {
		field := typ.FieldByName(key)
		if !field.IsValid() {
			return fmt.Errorf("type %T does not have expected form file field '%s'", dst, key)
		}

		if len(headers) == 0 {
			continue
		}

		if len(headers) > 1 {
			return fmt.Errorf("cannot parse multiple form file fields '%s' into slice field '%T.%s", key, typ, key)
		}

		r, err := headers[0].Open()
		if err != nil {
			return fmt.Errorf("cannot read file '%s': %w", key, err)
		}

		buf, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("cannot read full file '%s': %w", key, err)
		}

		field.SetBytes(buf)
	}
	return nil
}
