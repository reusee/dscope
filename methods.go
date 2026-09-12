package dscope

import (
	"errors"
	"fmt"
	"reflect"
)

// TheoryOfModuleMethodDiscovery documents how Methods expands a module object
// into provider functions and which inputs are rejected.
const TheoryOfModuleMethodDiscovery = `
dscope module method discovery theory:
- Method discovery expands a module object into provider functions: the
  exported methods of the object, of its pointer-chain targets, and of its
  module-typed fields are collected recursively.
- A method promoted into an enclosing type by an embedded module is collected
  once: discovery descends into the embedded module only for the module-typed
  fields it holds, skipping every method already collected with the same name
  and signature. Composing modules by embedding therefore never provides the
  same type twice.
- A struct passed by value is made addressable first so methods with pointer
  receivers are discovered; a typed nil pointer on a chain is materialised
  unless the chain ends in an interface.
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
			panic(errors.Join(
				fmt.Errorf("recursive pointer type %v", typ),
				ErrBadArgument,
			))
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
// collected one is a Go-promoted duplicate of the same logical provider; a
// method with a different signature is shadowed, not promoted, and is kept.
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

func Methods(objects ...any) (ret []any) {
	visitedTypes := make(map[reflect.Type]bool)
	// skipMethodNames carries the method names and signatures already collected
	// from an enclosing embedded type: Go promotes those methods into the
	// enclosing method set, so collecting them again would define the same
	// provided type twice.
	var extend func(v reflect.Value, skipMethodNames map[string]reflect.Type)
	extend = func(v reflect.Value, skipMethodNames map[string]reflect.Type) {
		if !v.IsValid() {
			panic(errors.Join(
				fmt.Errorf("invalid value"),
				ErrBadArgument,
			))
		}

		// nil interface
		if v.Kind() == reflect.Interface && v.IsNil() {
			panic(errors.Join(
				fmt.Errorf("invalid value: nil interface %v", v.Type()),
				ErrBadArgument,
			))
		}

		t := v.Type()
		validateFiniteMethodsPointerChain(t)
		if visitedTypes[t] {
			return
		}
		visitedTypes[t] = true

		if t.Kind() == reflect.Pointer && v.IsNil() {
			// If it's a pointer chain eventually to an interface that's nil, we can't construct.
			base := t
			for base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if base.Kind() == reflect.Interface {
				panic(errors.Join(
					fmt.Errorf("invalid value: nil pointer to interface %v", t),
					ErrBadArgument,
				))
			}
			// Construct concrete object for typed nil pointers like (*MyStruct)(nil)
			v = reflect.New(t.Elem())
		}

		// The method names and signatures reachable at this level. They travel
		// down to embedded module fields, whose methods Go promotes here.
		names := make(map[string]reflect.Type, len(skipMethodNames)+v.NumMethod())
		for name, typ := range skipMethodNames {
			names[name] = typ
		}

		// method sets
		ret = append(ret, collectMethods(v, skipMethodNames, names)...)

		// from fields
		for t.Kind() == reflect.Pointer {
			// deref
			t = t.Elem()
			if v.IsNil() {
				// Allocate new instance for nil pointers to avoid panic on Elem()
				v = reflect.New(t).Elem()
			} else {
				v = v.Elem()
			}

			if t.Kind() == reflect.Pointer {
				// Collect methods from intermediate pointers (e.g. *T when we started with **T)
				ret = append(ret, collectMethods(v, skipMethodNames, names)...)
			}
		}
		if t.Kind() == reflect.Struct {
			for i := range t.NumField() {
				field := t.Field(i)
				if field.PkgPath != "" {
					continue
				}
				if field.Type.Implements(isModuleType) {
					fv := v.Field(i)
					// An embedded module contributes its methods to the
					// enclosing method set, so discovery descends into it only
					// for the module-typed fields it holds. A named module
					// field promotes nothing and contributes all its methods.
					var skip map[string]reflect.Type
					if field.Anonymous {
						skip = names
					}
					if fv.Kind() == reflect.Struct {
						if fv.CanAddr() {
							extend(fv.Addr(), skip)
						} else {
							// For non-addressable struct fields (e.g. when the parent is passed by value),
							// we create an addressable copy to ensure methods with pointer receivers are found.
							ptr := reflect.New(fv.Type())
							ptr.Elem().Set(fv)
							extend(ptr, skip)
						}
					} else {
						extend(fv, skip)
					}
				}
			}
		}
	}

	for _, object := range objects {
		v := reflect.ValueOf(object)
		if v.IsValid() && v.Kind() == reflect.Struct {
			// For root structs passed by value, create an addressable copy
			// to ensure pointer receiver methods are discovered.
			ptr := reflect.New(v.Type())
			ptr.Elem().Set(v)
			v = ptr
		}
		extend(v, nil)
	}

	return
}
