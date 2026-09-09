package goslogx

import (
	"bytes"
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
		"private_key", "api_secret", "cookie",
	}
)

// structMetaCache caches struct metadata to avoid repeated reflection.
// Key: reflect.Type, Value: *structMeta
var structMetaCache sync.Map

var jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

var maskedObjectPool = sync.Pool{
	New: func() any { return new(maskedObject) },
}

var maskedArrayPool = sync.Pool{
	New: func() any { return new(maskedArray) },
}

var maskJSONBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

const maxPooledMaskJSONBuffer = 64 << 10

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
	v    any
	mode maskType
}

// MarshalLogObject lets the DTOs encode themselves without allocating an
// additional maskedObject wrapper on every log call.
func (data HTTPData) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return marshalHTTPData(enc, &data)
}

func (data DBData) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return marshalDBData(enc, &data)
}

func (data MQData) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return marshalMQData(enc, &data)
}

func (data GenericData) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return marshalGenericData(enc, &data)
}

// MarshalLogObject implements zapcore.ObjectMarshaler.
// It marshals struct fields with automatic masking based on struct tags.
// Uses cached struct metadata to minimize reflection overhead.
func (m maskedObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	switch value := m.v.(type) {
	case HTTPData:
		return marshalHTTPData(enc, &value)
	case *HTTPData:
		if value == nil {
			return nil
		}
		return marshalHTTPData(enc, value)
	case DBData:
		return marshalDBData(enc, &value)
	case *DBData:
		if value == nil {
			return nil
		}
		return marshalDBData(enc, value)
	case MQData:
		return marshalMQData(enc, &value)
	case *MQData:
		if value == nil {
			return nil
		}
		return marshalMQData(enc, value)
	case GenericData:
		return marshalGenericData(enc, &value)
	case *GenericData:
		if value == nil {
			return nil
		}
		return marshalGenericData(enc, value)
	case map[string][]string:
		if m.mode == maskHeaders {
			return marshalMaskedHeaders(enc, value)
		}
	}

	rv := reflect.ValueOf(m.v)
	for rv.IsValid() && (rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface) {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil
	}
	if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
		return marshalMaskedMap(enc, rv, m.mode)
	}
	if rv.Kind() != reflect.Struct {
		return enc.AddReflected("value", rv.Interface())
	}
	// Get cached metadata (zero reflection after first call)
	meta := getStructMeta(rv.Type())
	// Marshal each field
	for _, f := range meta.fields {
		fv := rv.Field(f.index)
		if m.mode == maskAuto && (f.skip || (f.omitEmpty && fv.IsZero())) {
			continue
		}
		if f.mask == maskAuto {
			if err := addAutoPayload(enc, f.name, fv); err != nil {
				return err
			}
			continue
		}
		if f.mask == maskHeaders {
			if err := addMaskedValue(enc, f.name, fv, maskHeaders, maskNone); err != nil {
				return err
			}
			continue
		}
		fieldMask := f.mask
		if m.mode == maskAuto && fieldMask == maskNone {
			fieldMask = shouldMaskField(f.name)
		}
		if m.mode == maskAuto {
			if err := addMaskedValue(enc, f.name, fv, maskAuto, fieldMask); err != nil {
				return err
			}
			continue
		}
		// Handle nested structs recursively
		if f.kind == reflect.Struct {
			if f.isTime {
				// Special case: time.Time
				enc.AddTime(f.name, fv.Interface().(time.Time))
				continue
			}
			// Recursively marshal nested struct
			if err := addPooledObject(enc, f.name, fv.Interface(), maskNone); err != nil {
				return err
			}
			continue
		}
		// Handle pointer to struct
		if f.kind == reflect.Ptr && !fv.IsNil() {
			elem := fv.Elem()
			if elem.Kind() == reflect.Struct {
				if err := addPooledObject(enc, f.name, elem.Interface(), maskNone); err != nil {
					return err
				}
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
					if err := addPooledArray(enc, f.name, fv, nil, maskNone); err != nil {
						return err
					}
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

func marshalHTTPData(enc zapcore.ObjectEncoder, data *HTTPData) error {
	enc.AddString("method", data.Method)
	enc.AddString("url", data.URL)
	enc.AddInt("status_code", data.StatusCode)
	if data.Headers == nil {
		if err := enc.AddReflected("headers", nil); err != nil {
			return err
		}
	} else if err := addPooledObject(enc, "headers", data.Headers, maskHeaders); err != nil {
		return err
	}
	if err := addAutoPayload(enc, "body", reflect.ValueOf(data.Body)); err != nil {
		return err
	}
	enc.AddString("duration", data.Duration)
	enc.AddString("client_ip", data.ClientIP)
	return nil
}

func marshalDBData(enc zapcore.ObjectEncoder, data *DBData) error {
	enc.AddString("driver", data.Driver)
	enc.AddString("operation", data.Operation)
	enc.AddString("database", data.Database)
	enc.AddString("table", data.Table)
	enc.AddString("statement", data.Statement)
	enc.AddString("duration", data.Duration)
	return addAutoPayload(enc, "payload", reflect.ValueOf(data.Payload))
}

func marshalMQData(enc zapcore.ObjectEncoder, data *MQData) error {
	enc.AddString("driver", data.Driver)
	enc.AddString("operation", data.Operation)
	enc.AddString("topic", data.Topic)
	enc.AddString("group", data.Group)
	enc.AddString("message_id", data.MessageID)
	return addAutoPayload(enc, "payload", reflect.ValueOf(data.Payload))
}

func marshalGenericData(enc zapcore.ObjectEncoder, data *GenericData) error {
	enc.AddString("service", data.Service)
	enc.AddString("action", data.Action)
	return addAutoPayload(enc, "payload", reflect.ValueOf(data.Payload))
}

func marshalMaskedHeaders(enc zapcore.ObjectEncoder, headers map[string][]string) error {
	for key, values := range headers {
		if err := addPooledArray(enc, key, reflect.Value{}, values, shouldMaskField(key)); err != nil {
			return err
		}
	}
	return nil
}

// maskedArray wraps a slice/array for custom marshaling with masking support.
type maskedArray struct {
	v       reflect.Value
	strings []string
	mode    maskType
}

// MarshalLogArray implements zapcore.ArrayMarshaler.
// It marshals array elements with automatic masking for structs.
func (m maskedArray) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	if m.strings != nil {
		for _, value := range m.strings {
			switch m.mode {
			case maskFull:
				value = "****"
			case maskPartial:
				value = maskMiddle(value)
			}
			enc.AppendString(value)
		}
		return nil
	}
	for i := 0; i < m.v.Len(); i++ {
		if m.mode == maskNone {
			elem := m.v.Index(i)
			if elem.Kind() == reflect.Pointer {
				if elem.IsNil() {
					if err := enc.AppendReflected(nil); err != nil {
						return err
					}
					continue
				}
				elem = elem.Elem()
			}
			if elem.Kind() == reflect.Struct {
				if err := appendPooledObject(enc, elem.Interface(), maskNone); err != nil {
					return err
				}
			} else if err := enc.AppendReflected(elem.Interface()); err != nil {
				return err
			}
			continue
		}
		if err := appendMaskedValue(enc, m.v.Index(i), m.mode); err != nil {
			return err
		}
	}
	return nil
}

func addPooledObject(enc zapcore.ObjectEncoder, key string, value any, mode maskType) error {
	object := maskedObjectPool.Get().(*maskedObject)
	object.v = value
	object.mode = mode
	err := enc.AddObject(key, object)
	object.v = nil
	object.mode = maskNone
	maskedObjectPool.Put(object)
	return err
}

func appendPooledObject(enc zapcore.ArrayEncoder, value any, mode maskType) error {
	object := maskedObjectPool.Get().(*maskedObject)
	object.v = value
	object.mode = mode
	err := enc.AppendObject(object)
	object.v = nil
	object.mode = maskNone
	maskedObjectPool.Put(object)
	return err
}

func addPooledArray(enc zapcore.ObjectEncoder, key string, value reflect.Value, stringsValue []string, mode maskType) error {
	array := maskedArrayPool.Get().(*maskedArray)
	array.v = value
	array.strings = stringsValue
	array.mode = mode
	err := enc.AddArray(key, array)
	array.v = reflect.Value{}
	array.strings = nil
	array.mode = maskNone
	maskedArrayPool.Put(array)
	return err
}

func appendPooledArray(enc zapcore.ArrayEncoder, value reflect.Value, mode maskType) error {
	array := maskedArrayPool.Get().(*maskedArray)
	array.v = value
	array.mode = mode
	err := enc.AppendArray(array)
	array.v = reflect.Value{}
	array.mode = maskNone
	maskedArrayPool.Put(array)
	return err
}

func marshalMaskedMap(enc zapcore.ObjectEncoder, rv reflect.Value, mode maskType) error {
	iter := rv.MapRange()
	for iter.Next() {
		key := iter.Key().String()
		fieldMask := maskNone
		if mode == maskAuto || mode == maskHeaders {
			fieldMask = shouldMaskField(key)
		}
		if err := addMaskedValue(enc, key, iter.Value(), mode, fieldMask); err != nil {
			return err
		}
	}
	return nil
}

// addAutoPayload handles the value stored directly in a masked:auto field.
// JSON strings and bytes retain their existing string output; structured Go
// values are encoded directly to Zap to avoid a marshal/decode round trip.
func addAutoPayload(enc zapcore.ObjectEncoder, key string, rv reflect.Value) error {
	value := rv
	for value.IsValid() && value.Kind() == reflect.Interface {
		if value.IsNil() {
			return enc.AddReflected(key, nil)
		}
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return enc.AddReflected(key, nil)
		}
		if value.Type().Implements(jsonMarshalerType) {
			masked, err := maskPayload(value.Interface())
			if err != nil {
				return err
			}
			return enc.AddReflected(key, masked)
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return enc.AddReflected(key, nil)
	}
	if value.Kind() == reflect.String {
		enc.AddString(key, maskJSONString(value.String()))
		return nil
	}
	if value.Type() == reflect.TypeOf([]byte(nil)) {
		if value.IsNil() {
			return enc.AddReflected(key, nil)
		}
		enc.AddString(key, maskJSONString(string(value.Bytes())))
		return nil
	}
	if value.Type() == reflect.TypeOf(json.RawMessage(nil)) {
		masked, err := maskPayload(value.Interface())
		if err != nil {
			return err
		}
		return enc.AddReflected(key, masked)
	}
	return addMaskedValue(enc, key, value, maskAuto, maskNone)
}

func addMaskedValue(enc zapcore.ObjectEncoder, key string, rv reflect.Value, mode, fieldMask maskType) error {
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return enc.AddReflected(key, nil)
		}
		rv = rv.Elem()
	}
	if rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return enc.AddReflected(key, nil)
		}
		if mode == maskAuto && rv.Type().Implements(jsonMarshalerType) {
			value, err := maskPayload(rv.Interface())
			if err != nil {
				return err
			}
			return enc.AddReflected(key, value)
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return enc.AddReflected(key, nil)
	}
	if (rv.Kind() == reflect.Map || rv.Kind() == reflect.Slice) && rv.IsNil() {
		return enc.AddReflected(key, nil)
	}
	if mode == maskAuto && rv.CanInterface() && rv.Type().Implements(jsonMarshalerType) {
		value, err := maskPayload(rv.Interface())
		if err != nil {
			return err
		}
		return enc.AddReflected(key, value)
	}

	if mode == maskHeaders && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) {
		return addPooledArray(enc, key, rv, nil, fieldMask)
	}
	if rv.Kind() == reflect.String {
		value := rv.String()
		switch fieldMask {
		case maskFull:
			value = "****"
		case maskPartial:
			value = maskMiddle(value)
		}
		enc.AddString(key, value)
		return nil
	}

	switch rv.Kind() {
	case reflect.Struct:
		if rv.Type() == reflect.TypeOf(time.Time{}) {
			enc.AddTime(key, rv.Interface().(time.Time))
			return nil
		}
		return addPooledObject(enc, key, rv.Interface(), mode)
	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			return addPooledObject(enc, key, rv.Interface(), mode)
		}
	case reflect.Slice, reflect.Array:
		return addPooledArray(enc, key, rv, nil, mode)
	case reflect.Bool:
		enc.AddBool(key, rv.Bool())
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		enc.AddInt64(key, rv.Int())
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		enc.AddUint64(key, rv.Uint())
		return nil
	case reflect.Float32, reflect.Float64:
		enc.AddFloat64(key, rv.Float())
		return nil
	}

	value, err := maskPayload(rv.Interface())
	if err != nil {
		return err
	}
	return enc.AddReflected(key, value)
}

func appendMaskedValue(enc zapcore.ArrayEncoder, rv reflect.Value, mode maskType) error {
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return enc.AppendReflected(nil)
		}
		rv = rv.Elem()
	}
	if rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return enc.AppendReflected(nil)
		}
		if mode == maskAuto && rv.Type().Implements(jsonMarshalerType) {
			value, err := maskPayload(rv.Interface())
			if err != nil {
				return err
			}
			return enc.AppendReflected(value)
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return enc.AppendReflected(nil)
	}
	if (rv.Kind() == reflect.Map || rv.Kind() == reflect.Slice) && rv.IsNil() {
		return enc.AppendReflected(nil)
	}
	if mode == maskAuto && rv.CanInterface() && rv.Type().Implements(jsonMarshalerType) {
		value, err := maskPayload(rv.Interface())
		if err != nil {
			return err
		}
		return enc.AppendReflected(value)
	}
	if rv.Kind() == reflect.String {
		value := rv.String()
		switch mode {
		case maskFull:
			value = "****"
		case maskPartial:
			value = maskMiddle(value)
		}
		enc.AppendString(value)
		return nil
	}
	switch rv.Kind() {
	case reflect.Struct:
		if rv.Type() == reflect.TypeOf(time.Time{}) {
			enc.AppendTime(rv.Interface().(time.Time))
			return nil
		}
		return appendPooledObject(enc, rv.Interface(), mode)
	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			return appendPooledObject(enc, rv.Interface(), mode)
		}
	case reflect.Slice, reflect.Array:
		return appendPooledArray(enc, rv, mode)
	case reflect.Bool:
		enc.AppendBool(rv.Bool())
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		enc.AppendInt64(rv.Int())
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		enc.AppendUint64(rv.Uint())
		return nil
	case reflect.Float32, reflect.Float64:
		enc.AppendFloat64(rv.Float())
		return nil
	}
	value, err := maskPayload(rv.Interface())
	if err != nil {
		return err
	}
	return enc.AppendReflected(value)
}

// fieldMeta contains cached metadata for a single struct field.
type fieldMeta struct {
	name      string       // Field name (for JSON key)
	index     int          // Field index in struct
	kind      reflect.Kind // Field type kind
	mask      maskType     // Masking strategy
	isTime    bool         // True if field is time.Time
	omitEmpty bool         // True when the JSON tag contains omitempty
	skip      bool         // True when the JSON tag is "-"
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
	maskAuto                    // Recursive field-name masking for DTO payloads
	maskHeaders                 // HTTP header masking
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
		skip := jsonTag == "-"
		omitEmpty := false
		if jsonTag != "" && !skip {
			// Parse JSON tag (handle "name,omitempty" format)
			if idx := strings.IndexByte(jsonTag, ','); idx >= 0 {
				if idx > 0 {
					fieldName = jsonTag[:idx]
				}
				omitEmpty = strings.Contains(jsonTag[idx+1:], "omitempty")
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
		case "masked:auto":
			mt = maskAuto
		case "masked:headers":
			mt = maskHeaders
		}
		// Check if field is time.Time
		isTime := f.Type == reflect.TypeOf(time.Time{})
		m.fields = append(m.fields, fieldMeta{
			name:      fieldName, // Use JSON tag name
			index:     i,
			kind:      f.Type.Kind(),
			mask:      mt,
			isTime:    isTime,
			omitEmpty: omitEmpty,
			skip:      skip,
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
	// Check for full masking patterns first (higher priority)
	for _, pattern := range fullMaskFields {
		if containsFoldNormalized(fieldName, pattern) {
			return maskFull
		}
	}
	// Check for partial masking patterns
	for _, pattern := range partialMaskFields {
		if containsFoldNormalized(fieldName, pattern) {
			return maskPartial
		}
	}
	return maskNone
}

// containsFoldNormalized matches the lowercase patterns used by the masking
// rules without allocating a normalized copy of the field name.
func containsFoldNormalized(fieldName, pattern string) bool {
	if len(pattern) > len(fieldName) {
		return false
	}
	for start := 0; start <= len(fieldName)-len(pattern); start++ {
		matched := true
		for i := 0; i < len(pattern); i++ {
			c := fieldName[start+i]
			if c == '-' {
				c = '_'
			} else if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != pattern[i] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// maskJSONString parses a JSON string and masks sensitive fields.
// Returns the original string if parsing fails.
func maskJSONString(jsonStr string) string {
	if jsonStr == "" {
		return jsonStr
	}
	jsonBytes := []byte(jsonStr)
	if !json.Valid(jsonBytes) {
		return jsonStr
	}
	// Escaped strings and HTML-sensitive characters use encoding/json so their
	// canonical escaping remains identical to the previous implementation.
	if strings.ContainsAny(jsonStr, `\<>&`) || !isASCII(jsonStr) {
		return maskJSONStringReflected(jsonStr)
	}

	buf := maskJSONBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Grow(len(jsonStr))
	index := 0
	writeMaskedJSONValue(buf, jsonStr, &index)
	if buf.Len() == len(jsonBytes) && bytes.Equal(buf.Bytes(), jsonBytes) {
		releaseMaskJSONBuffer(buf)
		return jsonStr
	}
	result := buf.String()
	releaseMaskJSONBuffer(buf)
	return result
}

func releaseMaskJSONBuffer(buf *bytes.Buffer) {
	if buf.Cap() > maxPooledMaskJSONBuffer {
		return
	}
	buf.Reset()
	maskJSONBufferPool.Put(buf)
}

func isASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] >= 0x80 {
			return false
		}
	}
	return true
}

func maskJSONStringReflected(jsonStr string) string {
	var data interface{}
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
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

func skipJSONSpace(src string, index *int) {
	for *index < len(src) {
		switch src[*index] {
		case ' ', '\n', '\r', '\t':
			*index++
		default:
			return
		}
	}
}

func scanJSONString(src string, index *int) (start, end int) {
	start = *index
	*index++
	for src[*index] != '"' {
		*index++
	}
	*index++
	return start, *index
}

func writeMaskedJSONValue(buf *bytes.Buffer, src string, index *int) {
	skipJSONSpace(src, index)
	switch src[*index] {
	case '{':
		writeMaskedJSONObject(buf, src, index)
	case '[':
		writeMaskedJSONArray(buf, src, index)
	case '"':
		start, end := scanJSONString(src, index)
		buf.WriteString(src[start:end])
	default:
		start := *index
		for *index < len(src) {
			c := src[*index]
			if c == ',' || c == '}' || c == ']' || c == ' ' || c == '\n' || c == '\r' || c == '\t' {
				break
			}
			*index++
		}
		buf.WriteString(src[start:*index])
	}
}

func writeMaskedJSONObject(buf *bytes.Buffer, src string, index *int) {
	buf.WriteByte('{')
	*index++
	skipJSONSpace(src, index)
	if src[*index] == '}' {
		buf.WriteByte('}')
		*index++
		return
	}
	for {
		skipJSONSpace(src, index)
		keyStart, keyEnd := scanJSONString(src, index)
		buf.WriteString(src[keyStart:keyEnd])
		skipJSONSpace(src, index)
		buf.WriteByte(':')
		*index++
		skipJSONSpace(src, index)
		fieldMask := shouldMaskField(src[keyStart+1 : keyEnd-1])
		if src[*index] == '"' && fieldMask != maskNone {
			valueStart, valueEnd := scanJSONString(src, index)
			if fieldMask == maskFull || valueEnd-valueStart <= 6 {
				buf.WriteString(`"****"`)
			} else {
				buf.WriteByte('"')
				buf.WriteString(src[valueStart+1 : valueStart+3])
				buf.WriteString("****")
				buf.WriteString(src[valueEnd-3 : valueEnd-1])
				buf.WriteByte('"')
			}
		} else {
			writeMaskedJSONValue(buf, src, index)
		}
		skipJSONSpace(src, index)
		if src[*index] == '}' {
			buf.WriteByte('}')
			*index++
			return
		}
		buf.WriteByte(',')
		*index++
	}
}

func writeMaskedJSONArray(buf *bytes.Buffer, src string, index *int) {
	buf.WriteByte('[')
	*index++
	skipJSONSpace(src, index)
	if src[*index] == ']' {
		buf.WriteByte(']')
		*index++
		return
	}
	for {
		writeMaskedJSONValue(buf, src, index)
		skipJSONSpace(src, index)
		if src[*index] == ']' {
			buf.WriteByte(']')
			*index++
			return
		}
		buf.WriteByte(',')
		*index++
	}
}

// maskPayload normalizes structured payloads to JSON without changing caller-owned
// values. JSON strings and bytes use the same behavior as the public helpers.
// Encoding errors are returned rather than falling back to unmasked data.
func maskPayload(value any) (any, error) {
	switch v := value.(type) {
	case string:
		return maskJSONString(v), nil
	case []byte:
		if v == nil {
			return nil, nil
		}
		return maskJSONString(string(v)), nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return maskJSONValue(decoded), nil
}

// maskJSONValue recursively masks sensitive fields in JSON data.
func maskJSONValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return maskJSONMap(v)
	case []interface{}:
		for i := range v {
			v[i] = maskJSONValue(v[i])
		}
		return v
	default:
		return v
	}
}

// maskJSONMap masks a decoded JSON object in place. The decoded object is
// private to the masking call, so reusing it avoids a second map allocation.
func maskJSONMap(data map[string]interface{}) map[string]interface{} {
	for key, value := range data {
		maskType := shouldMaskField(key)
		switch v := value.(type) {
		case string:
			if maskType == maskFull {
				data[key] = "****"
			} else if maskType == maskPartial {
				data[key] = maskMiddle(v)
			}
		case map[string]interface{}:
			maskJSONMap(v)
		case []interface{}:
			maskJSONValue(v)
		}
	}
	return data
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
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return zap.Skip()
	}
	// Fast path: type switch for common types and ObjectMarshaler
	switch val := v.(type) {
	case zapcore.ObjectMarshaler:
		return zap.Object(key, val)
	}
	// Slow path: use reflection for unknown types
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
				return zap.Array(key, maskedArray{v: rv})
			}
		}
		// For empty slices or primitive slices, use zap.Any
		return zap.Any(key, v)
	}
	// If it's a struct, wrap it with maskedObject
	if rv.Kind() == reflect.Struct {
		return zap.Object(key, maskedObject{v: rv.Interface()})
	}
	// For maps, use zap.Any (will be reflected)
	if rv.Kind() == reflect.Map {
		return zap.Any(key, v)
	}
	// For other types (primitives, etc), use zap.Any
	return zap.Any(key, v)
}
