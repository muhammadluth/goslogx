package mongo

import (
	"reflect"
	"strings"
	"testing"
)

func TestConnectionKeyFieldsHaveJSONTags(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(ConnectionKey{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		name, ok := field.Tag.Lookup("json")
		if !ok || strings.TrimSpace(name) == "" {
			t.Errorf("%s.%s must declare a json tag", typ.Name(), field.Name)
		}
	}
}
