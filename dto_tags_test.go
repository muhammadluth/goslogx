package goslogx

import (
	"reflect"
	"strings"
	"testing"
)

func TestPublicDTOFieldsHaveJSONTags(t *testing.T) {
	t.Parallel()

	assertJSONTags(t,
		HTTPData{},
		DBData{},
		MQData{},
		GenericData{},
	)
}

func assertJSONTags(t *testing.T, values ...any) {
	t.Helper()

	for _, value := range values {
		typ := reflect.TypeOf(value)
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
}
