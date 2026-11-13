package dscope

import "reflect"

// TheoryOfModuleMethodDiscovery documents how Methods expands a module object
// into provider functions and which inputs are rejected.
const TheoryOfModuleMethodDiscovery = `
dscope module method discovery theory:
- Each root object has its own discovery state: distinct roots of the same type
  each contribute bound methods, while pointer levels and module fields inside
  one root share one visit set.
- A method promoted into an enclosing type by an embedded module is collected
  once: discovery descends into the embedded module only for the module-typed
  fields it holds, skipping every method already collected with the same name
  and signature. Composing modules by embedding therefore never provides the
  same type twice.
- A struct passed by value is made addressable first so methods with pointer
  receivers are discovered; a typed nil pointer on a chain is materialised
  unless the chain ends in an interface.
- Pointer levels of one object are visited once, so recursive module fields do
  not collect the same intermediate pointer method set twice.
- A terminal non-nil interface follows its dynamic value; methods are
  discovered from that receiver, not from the interface wrapper. A nil terminal
  interface, or a nil interface at the end of a pointer chain, is a bad
  argument naming the original pointer type.
- Discovery accepts finite pointer chains; recursive pointer types are invalid
  inputs because they have no concrete terminal value from which module fields
  can be discovered.
- Invalid discovery inputs produce structured dscope errors and must never
  cause unbounded traversal.
`

func validateFiniteMethodsPointerChain(typ reflect.Type) {
	visitedTypes := make(map[reflect.Type]struct{})
	for typ.Kind() == reflect.Pointer {
		if _, exists := visitedTypes[typ]; exists {
			panic(errWith(ErrBadArgument, "recursive pointer type %v", typ))
		}
		visitedTypes[typ] = struct{}{}
		typ = typ.Elem()
	}
}

// sameMethodSignature reports whether two method types describe the same
// receiver-free signature. A method type carries its receiver as the first
// parameter, and a promoted method changes the receiver while keeping its
// signature, so the receiver must be excluded from the comparison.
func sameMethodSignature(a, b reflect.Type) bool {
	if a.NumIn() != b.NumIn() || a.NumOut() != b.NumOut() || a.IsVariadic() != b.IsVariadic() {
		return false
	}
	for i := 1; i < a.NumIn(); i++ {
		if a.In(i) != b.In(i) {
			return false
		}
	}
	for i := range a.NumOut() {
		if a.Out(i) != b.Out(i) {
			return false
		}
	}
	return true
}

// collectMethods appends the methods of v that are not already collected from
// an enclosing embedded type, and records every method name and signature of
// this level into names. A method with the same name and signature as a
// collected one is a Go-promoted duplicate of the same logical provider;
// a method with a different signature is shadowed, not promoted, and is kept.
func collectMethods(v reflect.Value, skip map[string]reflect.Type, names map[string]reflect.Type) []any {
	t := v.Type()
	var ret []any
	for i := range v.NumMethod() {
		method := t.Method(i)
		names[method.Name] = method.Type
		if typ, ok := skip[method.Name]; ok && sameMethodSignature(typ, method.Type) {
			continue
		}
		ret = append(ret, v.Method(i).Interface())
	}
	return ret
}

func methodReceiver(v reflect.Value) reflect.Value {
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return v
	}
	if v.CanAddr() {
		return v.Addr()
	}
	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)
	return ptr
}

func makeMethodNames(v reflect.Value, skip map[string]reflect.Type) map[string]reflect.Type {
	names := make(map[string]reflect.Type, len(skip)+v.NumMethod())
	for name, typ := range skip {
		names[name] = typ
	}
	return names
}

type _MethodDiscovery struct {
	visitedTypes map[reflect.Type]bool
	ret          []any
}

func (d *_MethodDiscovery) extend(v reflect.Value, skip map[string]reflect.Type) {
	d.extendWithOuter(v, skip, nil)
}

func (d *_MethodDiscovery) extendInterface(v reflect.Value, skip map[string]reflect.Type) {
	d.extend(methodReceiver(v.Elem()), skip)
}

func (d *_MethodDiscovery) extendStruct(v reflect.Value, t reflect.Type, names map[string]reflect.Type) {
	if t.Kind() != reflect.Struct {
		return
	}
	for i := range t.NumField() {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if !field.Type.Implements(isModuleType) {
			continue
		}
		var skip map[string]reflect.Type
		if field.Anonymous {
			skip = names
		}
		d.extend(methodReceiver(v.Field(i)), skip)
	}
}

func (d *_MethodDiscovery) extendPointerChain(
	v reflect.Value,
	t reflect.Type,
	skip map[string]reflect.Type,
	names map[string]reflect.Type,
	outerType reflect.Type,
) {
	base := t
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	endsInInterface := base.Kind() == reflect.Interface

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		if v.IsNil() {
			if endsInInterface {
				panic(errWith(ErrBadArgument, "nil pointer to interface %v", outerType))
			}
			v = reflect.New(t).Elem()
		} else {
			v = v.Elem()
		}
		if v.Kind() == reflect.Pointer && v.IsNil() {
			if endsInInterface {
				panic(errWith(ErrBadArgument, "nil pointer to interface %v", outerType))
			}
			v = reflect.New(v.Type().Elem())
		}
		if t.Kind() == reflect.Pointer {
			collected := collectMethods(v, skip, names)
			if !d.visitedTypes[v.Type()] {
				d.visitedTypes[v.Type()] = true
				d.ret = append(d.ret, collected...)
			}
			continue
		}
		if t.Kind() == reflect.Interface {
			if v.IsNil() {
				panic(errWith(ErrBadArgument, "nil pointer to interface %v", outerType))
			}
			d.extendInterface(v, names)
			return
		}
	}
	d.extendStruct(v, t, names)
}

func (d *_MethodDiscovery) extendWithOuter(
	v reflect.Value,
	skip map[string]reflect.Type,
	outerType reflect.Type,
) {
	if !v.IsValid() {
		panic(errWith(ErrBadArgument, "invalid value"))
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			if outerType != nil {
				panic(errWith(ErrBadArgument, "nil pointer to interface %v", outerType))
			}
			panic(errWith(ErrBadArgument, "invalid value: nil interface %v", v.Type()))
		}
		d.extendInterface(v, skip)
		return
	}

	t := v.Type()
	validateFiniteMethodsPointerChain(t)
	if d.visitedTypes[t] {
		return
	}
	d.visitedTypes[t] = true
	if outerType == nil {
		outerType = t
	}
	if t.Kind() == reflect.Pointer && v.IsNil() {
		base := t
		for base.Kind() == reflect.Pointer {
			base = base.Elem()
		}
		if base.Kind() == reflect.Interface {
			panic(errWith(ErrBadArgument, "nil pointer to interface %v", outerType))
		}
		v = reflect.New(t.Elem())
	}

	names := makeMethodNames(v, skip)
	d.ret = append(d.ret, collectMethods(v, skip, names)...)
	d.extendPointerChain(v, t, skip, names, outerType)
}

func Methods(objects ...any) (ret []any) {
	for _, object := range objects {
		discovery := &_MethodDiscovery{
			visitedTypes: make(map[reflect.Type]bool),
		}
		discovery.extend(methodReceiver(reflect.ValueOf(object)), nil)
		ret = append(ret, discovery.ret...)
	}
	return
}
