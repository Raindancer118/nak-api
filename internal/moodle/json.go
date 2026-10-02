package moodle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// J is a lenient view on decoded JSON: missing paths yield zero values, like
// Jackson's path() in the Java original.
type J struct{ v any }

func parseJ(b []byte) (J, error) {
	var v any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return J{}, err
	}
	return J{v}, nil
}

func (j J) Get(k string) J {
	if m, ok := j.v.(map[string]any); ok {
		return J{m[k]}
	}
	return J{}
}

func (j J) Idx(i int) J {
	if a, ok := j.v.([]any); ok && i >= 0 && i < len(a) {
		return J{a[i]}
	}
	return J{}
}

func (j J) Arr() []J {
	a, _ := j.v.([]any)
	out := make([]J, len(a))
	for i, x := range a {
		out[i] = J{x}
	}
	return out
}

func (j J) Len() int {
	switch t := j.v.(type) {
	case []any:
		return len(t)
	case map[string]any:
		return len(t)
	}
	return 0
}

func (j J) Has(k string) bool {
	m, ok := j.v.(map[string]any)
	if !ok {
		return false
	}
	_, ok = m[k]
	return ok
}

func (j J) IsObj() bool    { _, ok := j.v.(map[string]any); return ok }
func (j J) IsArr() bool    { _, ok := j.v.([]any); return ok }
func (j J) IsNull() bool   { return j.v == nil }
func (j J) IsNumber() bool { _, ok := j.v.(json.Number); return ok }
func (j J) IsString() bool { _, ok := j.v.(string); return ok }
func (j J) Raw() any       { return j.v }

func (j J) Str(def string) string {
	switch t := j.v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	}
	return def
}

func (j J) Float(def float64) float64 {
	switch t := j.v.(type) {
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f
		}
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
	case bool:
		if t {
			return 1
		}
		return 0
	}
	return def
}

func (j J) Int(def int64) int64 {
	switch t := j.v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return int64(f)
		}
	case string:
		if i, err := strconv.ParseInt(t, 10, 64); err == nil {
			return i
		}
	case bool:
		if t {
			return 1
		}
		return 0
	}
	return def
}

func (j J) Bool(def bool) bool {
	switch t := j.v.(type) {
	case bool:
		return t
	case json.Number:
		return t.String() != "0"
	case string:
		return t == "1" || t == "true"
	}
	return def
}

func (j J) MarshalJSON() ([]byte, error) { return json.Marshal(j.v) }

// M is an insertion-ordered JSON object that drops nil/empty values, keeping
// tool output compact (port of MoodleService.m()).
type M struct {
	keys []string
	vals map[string]any
}

func m(kv ...any) *M {
	o := &M{vals: map[string]any{}}
	for i := 0; i+1 < len(kv); i += 2 {
		o.Put(kv[i].(string), kv[i+1])
	}
	return o
}

func (o *M) Put(k string, v any) *M {
	if empty(v) {
		return o
	}
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
	return o
}

func (o *M) Get(k string) any { return o.vals[k] }
func (o *M) Keys() []string   { return o.keys }

func empty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case *string:
		return t == nil || *t == ""
	case []any:
		return len(t) == 0
	case []string:
		return len(t) == 0
	case []*M:
		return len(t) == 0
	case *M:
		return t == nil || len(t.keys) == 0
	case *int64:
		return t == nil
	case *bool:
		return t == nil
	case *float64:
		return t == nil
	}
	return false
}

func (o *M) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		vb, err := json.Marshal(o.vals[k])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// opt helpers turn "absent" into nil so M drops them.
func ptrTrue(b bool) *bool {
	if !b {
		return nil
	}
	t := true
	return &t
}

func ptrInt(i int64, keep bool) *int64 {
	if !keep {
		return nil
	}
	return &i
}

func num(d float64) string {
	if d == math.Trunc(d) && !math.IsInf(d, 0) {
		return strconv.FormatInt(int64(d), 10)
	}
	s := strconv.FormatFloat(d, 'f', 2, 64)
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	return s
}
