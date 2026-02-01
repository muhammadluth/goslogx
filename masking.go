package goslogx

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Sensitive field patterns for automatic masking in JSON bodies and headers.
// These patterns are matched case-insensitively against field names.
var (
	// partialMaskFields contains patterns for fields that should show first/last characters.
	partialMaskFields = []string{
		"username", "user_name", "email", "phone", "mobile",
		"access_key", "api_key", "client_id", "user_id",
	}
	// fullMaskFields contains patterns for fields that should be completely hidden.
	fullMaskFields = []string{
		"password", "passwd", "pwd", "secret", "secret_key",
		"token", "auth", "authorization", "bearer", "credential",
		"private_key", "api_secret",
	}
)

// structMetaCache caches struct metadata to avoid repeated reflection.
// Key: reflect.Type, Value: *structMeta
var structMetaCache sync.Map

// maskedObject wraps any value for custom marshaling with field masking support.
// It implements zapcore.ObjectMarshaler to provide efficient struct encoding
// with automatic masking of sensitive fields tagged with log:"masked:*".
//
// Supports:
//   - Nested structs (unlimited depth)
//   - Pointer types (nil-safe)
//   - All basic Go types (int, uint, float, bool, string)
//   - Special handling for time.Time
//   - Maps and slices (via reflection)
//
// Example:
//
//	type User struct {
//	    Email string `json:"email" log:"masked:partial"`
//	    Password string `json:"password" log:"masked:full"`
//	}
//	// Automatically masks when logged via goslogx.Info()
type maskedObject struct {
	v any
}

// MarshalLogObject implements zapcore.ObjectMarshaler.
// It marshals struct fields with automatic masking based on struct tags.
// Uses cached struct metadata to minimize reflection overhead.
func (m maskedObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	rv := reflect.ValueOf(m.v)
	// Handle pointers
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	// For non-struct types, we need special handling
	// This happens when dataField wraps slices/maps in maskedObject
	if rv.Kind() != reflect.Struct {
		// This shouldn't happen in normal flow since dataField
		// only wraps structs with maskedObject
		return enc.AddReflected("value", rv.Interface())
	}
	// Get cached metadata (zero reflection after first call)
	meta := getStructMeta(rv.Type())
	// Marshal each field
	for _, f := range meta.fields {
		fv := rv.Field(f.index)
		// Handle nested structs recursively
		if f.kind == reflect.Struct {
			if f.isTime {
				// Special case: time.Time
				enc.AddTime(f.name, fv.Interface().(time.Time))
				continue
			}
			// Recursively marshal nested struct
			enc.AddObject(f.name, maskedObject{fv.Interface()})
			continue
		}
		// Handle pointer to struct
		if f.kind == reflect.Ptr && !fv.IsNil() {
			elem := fv.Elem()
			if elem.Kind() == reflect.Struct {
				enc.AddObject(f.name, maskedObject{elem.Interface()})
				continue
			}
		}
		// Handle slices and arrays
		if f.kind == reflect.Slice || f.kind == reflect.Array {
			isNil := f.kind == reflect.Slice && fv.IsNil()
			if !isNil && fv.Len() > 0 {
				// Check if slice contains structs
				elemType := fv.Type().Elem()
				if elemType.Kind() == reflect.Struct || (elemType.Kind() == reflect.Ptr && elemType.Elem().Kind() == reflect.Struct) {
					// Slice of structs - use maskedArray for recursive masking
					enc.AddArray(f.name, maskedArray{fv})
					continue
				}
			}
			// Slice of primitives - use reflection
			enc.AddReflected(f.name, fv.Interface())
			continue
		}
		// Handle string fields with masking
		if f.kind == reflect.String {
			s := fv.String()
			switch f.mask {
			case maskFull:
				enc.AddString(f.name, "****")
			case maskPartial:
				enc.AddString(f.name, maskMiddle(s))
			default:
				enc.AddString(f.name, s)
			}
			continue
		}
		// Handle other basic types
		switch f.kind {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			enc.AddInt64(f.name, fv.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			enc.AddUint64(f.name, fv.Uint())
		case reflect.Float32, reflect.Float64:
			enc.AddFloat64(f.name, fv.Float())
		case reflect.Bool:
			enc.AddBool(f.name, fv.Bool())
		default:
			// Fallback for complex types
			enc.AddReflected(f.name, fv.Interface())
		}
	}
	return nil
}

// maskedArray wraps a slice/array for custom marshaling with masking support.
type maskedArray struct {
	v reflect.Value
}

// MarshalLogArray implements zapcore.ArrayMarshaler.
// It marshals array elements with automatic masking for structs.
func (m maskedArray) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for i := 0; i < m.v.Len(); i++ {
		elem := m.v.Index(i)
		// Handle pointer elements
		if elem.Kind() == reflect.Pointer {
			if elem.IsNil() {
				enc.AppendReflected(nil)
				continue
			}
			elem = elem.Elem()
		}
		// If element is a struct, wrap with maskedObject
		if elem.Kind() == reflect.Struct {
			enc.AppendObject(maskedObject{elem.Interface()})
		} else {
			enc.AppendReflected(elem.Interface())
		}
	}
	return nil
}

// fieldMeta contains cached metadata for a single struct field.
type fieldMeta struct {
	name   string       // Field name (for JSON key)
	index  int          // Field index in struct
	kind   reflect.Kind // Field type kind
	mask   maskType     // Masking strategy
	isTime bool         // True if field is time.Time
}

// structMeta contains cached metadata for all fields in a struct.
type structMeta struct {
	fields []fieldMeta
}

// maskType defines the masking strategy for a field.
type maskType uint8

const (
	maskNone    maskType = iota // No masking
	maskFull                    // Full masking: "****"
	maskPartial                 // Partial masking: show first 2 and last 2 chars
)

// getStructMeta retrieves or builds cached metadata for a struct type.
// Uses sync.Map for thread-safe caching.
// After the first call for a type, subsequent calls have zero reflection overhead.
func getStructMeta(t reflect.Type) *structMeta {
	// Check cache first
	if v, ok := structMetaCache.Load(t); ok {
		return v.(*structMeta)
	}
	// Build metadata
	m := &structMeta{
		fields: make([]fieldMeta, 0, t.NumField()),
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		// Skip unexported fields
		if !f.IsExported() {
			continue
		}
		// Get JSON tag name, default to field name
		jsonTag := f.Tag.Get("json")
		fieldName := f.Name
		if jsonTag != "" && jsonTag != "-" {
			// Parse JSON tag (handle "name,omitempty" format)
			if idx := strings.Index(jsonTag, ","); idx > 0 {
				fieldName = jsonTag[:idx]
			} else {
				fieldName = jsonTag
			}
		}
		// Parse masking tag
		tag := f.Tag.Get("log")
		mt := maskNone
		switch tag {
		case "masked:full":
			mt = maskFull
		case "masked:partial":
			mt = maskPartial
		}
		// Check if field is time.Time
		isTime := f.Type == reflect.TypeOf(time.Time{})
		m.fields = append(m.fields, fieldMeta{
			name:   fieldName, // Use JSON tag name
			index:  i,
			kind:   f.Type.Kind(),
			mask:   mt,
			isTime: isTime,
		})
	}
	// Cache for future use
	structMetaCache.Store(t, m)
	return m
}

// maskMiddle masks the middle portion of a string, showing only first 2 and last 2 characters.
//
// Examples:
//   - "johndoe123" → "jo****23"
//   - "john.doe@example.com" → "jo****om"
//   - "abc" → "****" (too short)
func maskMiddle(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}

// shouldMaskField determines if a field should be masked based on its name.
// Returns maskFull for sensitive fields (password, secret, token),
// maskPartial for identifiable fields (username, email), or maskNone.
func shouldMaskField(fieldName string) maskType {
	// Normalize: lowercase and replace dashes with underscores
	lower := strings.ToLower(strings.ReplaceAll(fieldName, "-", "_"))
	// Check for full masking patterns first (higher priority)
	for _, pattern := range fullMaskFields {
		if strings.Contains(lower, pattern) {
			return maskFull
		}
	}
	// Check for partial masking patterns
	for _, pattern := range partialMaskFields {
		if strings.Contains(lower, pattern) {
			return maskPartial
		}
	}
	return maskNone
}

// maskJSONString parses a JSON string and masks sensitive fields.
// Returns the original string if parsing fails.
func maskJSONString(jsonStr string) string {
	if jsonStr == "" {
		return jsonStr
	}
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// Not valid JSON, return as-is
		return jsonStr
	}
	masked := maskJSONValue(data)
	result, err := json.Marshal(masked)
	if err != nil {
		return jsonStr
	}
	return string(result)
}

// maskJSONValue recursively masks sensitive fields in JSON data.
func maskJSONValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return maskJSONMap(v)
	case []interface{}:
		// Handle arrays
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = maskJSONValue(item)
		}
		return result
	default:
		return v
	}
}

// maskJSONMap masks sensitive fields in a JSON object (map).
// Recursively processes nested objects.
func maskJSONMap(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range data {
		maskType := shouldMaskField(key)
		switch v := value.(type) {
		case string:
			if maskType == maskFull {
				result[key] = "****"
			} else if maskType == maskPartial {
				result[key] = maskMiddle(v)
			} else {
				result[key] = v
			}
		case map[string]interface{}:
			// Recursive for nested objects
			result[key] = maskJSONMap(v)
		case []interface{}:
			// Handle arrays
			result[key] = maskJSONValue(v)
		default:
			result[key] = v
		}
	}
	return result
}

// maskHttpHeaders masks sensitive values in HTTP headers or query parameters.
// Returns a new map with masked values.
func maskHttpHeaders(headers map[string][]string) map[string][]string {
	result := make(map[string][]string)
	for key, values := range headers {
		maskType := shouldMaskField(key)
		if maskType != maskNone && len(values) > 0 {
			masked := make([]string, len(values))
			for i, v := range values {
				if maskType == maskFull {
					masked[i] = "****"
				} else {
					masked[i] = maskMiddle(v)
				}
			}
			result[key] = masked
		} else {
			result[key] = values
		}
	}
	return result
}

// dataField creates a zap.Field for logging arbitrary data.
// Automatically wraps structs with maskedObject for field masking.
// Returns zap.Skip() for nil values.
//
// Behavior:
//   - nil → zap.Skip()
//   - zapcore.ObjectMarshaler → zap.Object()
//   - struct (direct or in interface{}) → zap.Object() with maskedObject wrapper
//   - slice/array → zap.Array() with maskedArray for struct elements
//   - other types → zap.Any()
func dataField(key string, v any) zap.Field {
	if v == nil {
		return zap.Skip()
	}
	// Fast path: type switch for common types and ObjectMarshaler
	switch val := v.(type) {
	case zapcore.ObjectMarshaler:
		return zap.Object(key, val)
	case HTTPData:
		// // Check if HTTPData needs masking
		// needsMasking := false
		// if val.Headers != nil {
		// 	for k := range val.Headers {
		// 		if shouldMaskField(k) != maskNone {
		// 			needsMasking = true
		// 			break
		// 		}
		// 	}
		// }
		// if !needsMasking && val.Body != nil {
		// 	if bodyStr, ok := val.Body.(string); ok && len(bodyStr) > 0 && (bodyStr[0] == '{' || bodyStr[0] == '[') {
		// 		needsMasking = true
		// 	}
		// }
		// if needsMasking {
		// 	return zap.Object(key, httpDataMasked{val})
		// }
		// // No masking needed, use default reflection (faster)
		// return zap.Any(key, val)
		return zap.Object(key, maskedObject{val})
	case *HTTPData:
		// if val == nil {
		// 	return zap.Skip()
		// }
		// // Check if HTTPData needs masking
		// needsMasking := false
		// if val.Headers != nil {
		// 	for k := range val.Headers {
		// 		if shouldMaskField(k) != maskNone {
		// 			needsMasking = true
		// 			break
		// 		}
		// 	}
		// }
		// if !needsMasking && val.Body != nil {
		// 	if bodyStr, ok := val.Body.(string); ok && len(bodyStr) > 0 && (bodyStr[0] == '{' || bodyStr[0] == '[') {
		// 		needsMasking = true
		// 	}
		// }
		// if needsMasking {
		// 	return zap.Object(key, httpDataMasked{*val})
		// }
		// return zap.Any(key, val)
		return zap.Object(key, maskedObject{val})
	case DBData:
		return zap.Object(key, maskedObject{val})
	case *DBData:
		return zap.Object(key, maskedObject{val})
	case MQData:
		return zap.Object(key, maskedObject{val})
	case *MQData:
		return zap.Object(key, maskedObject{val})
	case GenericData:
		return zap.Object(key, maskedObject{val})
	case *GenericData:
		return zap.Object(key, maskedObject{val})
	}
	// Slow path: use reflection for unknown types
	rv := reflect.ValueOf(v)
	// Handle pointer
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return zap.Skip()
		}
		rv = rv.Elem()
	}
	// Handle slices and arrays - directly as Array, not wrapped in Object
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		// Check if slice contains structs
		if rv.Len() > 0 {
			elemType := rv.Type().Elem()
			// Handle pointer to struct
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			// If elements are structs, use maskedArray for masking
			if elemType.Kind() == reflect.Struct {
				return zap.Array(key, maskedArray{rv})
			}
		}
		// For empty slices or primitive slices, use zap.Any
		return zap.Any(key, v)
	}
	// If it's a struct, wrap it with maskedObject
	if rv.Kind() == reflect.Struct {
		return zap.Object(key, maskedObject{rv.Interface()})
	}
	// For maps, use zap.Any (will be reflected)
	if rv.Kind() == reflect.Map {
		return zap.Any(key, v)
	}
	// For other types (primitives, etc), use zap.Any
	return zap.Any(key, v)
}
