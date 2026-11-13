package dscope

import (
	"reflect"
	"sync/atomic"
)

// TheoryOfScopeInjectStruct documents struct field injection: how fields are
// selected and resolved.
const TheoryOfScopeInjectStruct = `
dscope inject theory:
- InjectStruct fills the exported fields of a struct from the scope; it wires
  objects that need many dependencies without forcing every dependency to be
  a constructor parameter.
- A field is injected when it carries the tag dscope:"." or dscope:"inject";
  a field of type Inject[T] receives a lazy function that resolves T on
  demand; an untagged embedded struct is recursed into, allocating nil pointer
  fields along the way; a field tagged dscope:"-" is skipped.
- Tagged fields are resolved exactly like any other lookup. Unexported fields
  are never touched.
- Circular struct embedding is rejected before mutating the target.
- The target may be a pointer chain ending in a struct; nil pointers along the
  chain are allocated when settable.
`

type InjectStruct func(target any)

const injectStructTypeID _TypeID = 2

func throwNilInjectTarget() {
	panic(errWith(ErrBadArgument, "target must be a pointer to a struct, got nil"))
}

func throwNilPtrInjectTarget(targetType reflect.Type) {
	panic(errWith(ErrBadArgument, "cannot inject into a nil pointer target of type %v", targetType))
}

func (scope Scope) InjectStruct(target any) {
	if target == nil {
		throwNilInjectTarget()
	}
	targetType := reflect.TypeOf(target)
	if m := injectFastMemo.Load(); m != nil && m.scope.values == scope.values && m.typ == targetType && m.scope.signature == scope.signature {
		elem := reflect.ValueOf(target).Elem()
		if !elem.IsValid() {
			throwNilPtrInjectTarget(targetType)
		}
		f := elem.Field(m.fieldIndex)
		if m.isIntField {
			f.SetInt(m.intVal)
		} else {
			f.Set(m.rv)
		}
		return
	}
	injectStructSlow(scope, target, targetType, 0)
}

type _InjectStructFunc = func(scope Scope, value reflect.Value, depth int)

type _InjectFastMemo struct {
	typ        reflect.Type
	scope      Scope
	rv         reflect.Value
	intVal     int64
	fieldIndex int
	isIntField bool
}

var (
	injectFastMemo      atomic.Pointer[_InjectFastMemo]
	injectLastTypeEntry atomic.Pointer[_TypeIDCacheEntry]
)

func injectStruct(scope Scope, target any, depth int) {
	if target == nil {
		throwNilInjectTarget()
	}
	targetType := reflect.TypeOf(target)
	if depth == 0 {
		if m := injectFastMemo.Load(); m != nil && m.scope.values == scope.values && m.typ == targetType && m.scope.signature == scope.signature {
			elem := reflect.ValueOf(target).Elem()
			if !elem.IsValid() {
				throwNilPtrInjectTarget(targetType)
			}
			f := elem.Field(m.fieldIndex)
			if m.isIntField {
				f.SetInt(m.intVal)
			} else {
				f.Set(m.rv)
			}
			return
		}
	}
	injectStructSlow(scope, target, targetType, depth)
}

func injectStructSlow(scope Scope, target any, targetType reflect.Type, depth int) {
	if e := injectLastTypeEntry.Load(); e != nil && e.typ == targetType {
		if fn := e.injectFn.Load(); fn != nil {
			(*fn)(scope, reflect.ValueOf(target), depth)
			return
		}
	}
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer {
		panic(errWith(ErrBadArgument, "target must be a pointer to a struct, got %v", targetType))
	}
	entry := getTypeEntry(targetType)
	fnPtr := entry.injectFn.Load()
	if fnPtr == nil {
		created := makeInjectStructFunc(targetType)
		entry.injectFn.CompareAndSwap(nil, &created)
		fnPtr = entry.injectFn.Load()
	}
	injectLastTypeEntry.Store(entry)
	(*fnPtr)(scope, v, depth)
}

type _InjectFieldCache struct {
	scope  Scope
	rv     reflect.Value
	intVal int64
}

func validateAcyclicEmbeddedStructs(root reflect.Type) {
	states := make(map[reflect.Type]uint8)
	var visit func(t reflect.Type)
	visit = func(t reflect.Type) {
		switch states[t] {
		case 1:
			panic(errWith(ErrBadArgument, "recursive struct injection depth limit exceeded: %v", root))
		case 2:
			return
		}
		states[t] = 1
		for i := range t.NumField() {
			field := t.Field(i)
			if field.PkgPath != "" || !field.Anonymous {
				continue
			}
			if field.Tag.Get("dscope") != "" {
				continue
			}
			if field.Type.Kind() == reflect.Func && field.Type.Implements(isInjectType) {
				continue
			}
			ft := field.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() != reflect.Struct {
				continue
			}
			visit(ft)
		}
		states[t] = 2
	}
	visit(root)
}

func isSignedIntKind(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Int64
}

func setReflectedField(field reflect.Value, c *_InjectFieldCache, isInt bool) {
	if isInt {
		field.SetInt(c.intVal)
		return
	}
	field.Set(c.rv)
}

type _InjectFieldInfo struct {
	Index       int
	IsInject    bool
	IsEmbedded  bool
	IsSignedInt bool
	Maker       isInjectMaker
	Type        reflect.Type
	TypeID      _TypeID
	cache       atomic.Pointer[_InjectFieldCache]
}

func collectInjectFieldInfos(structType reflect.Type) []_InjectFieldInfo {
	var infos []_InjectFieldInfo
	for i := range structType.NumField() {
		field := structType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		directive := field.Tag.Get("dscope")
		if directive == "-" {
			continue
		}
		if field.Type.Kind() == reflect.Func && field.Type.Implements(isInjectType) {
			injectedType := field.Type.Out(0)
			maker, _ := reflect.Zero(field.Type).Interface().(isInjectMaker)
			infos = append(infos, _InjectFieldInfo{
				Index:    i,
				IsInject: true,
				Maker:    maker,
				Type:     injectedType,
				TypeID:   getTypeID(injectedType),
			})
		} else if directive == "." || directive == "inject" {
			infos = append(infos, _InjectFieldInfo{
				Index:       i,
				IsSignedInt: isSignedIntKind(field.Type.Kind()),
				Type:        field.Type,
				TypeID:      getTypeID(field.Type),
			})
		} else if field.Anonymous && directive == "" {
			fieldType := field.Type
			if fieldType.Kind() == reflect.Pointer {
				if fieldType.Elem().Kind() != reflect.Struct {
					continue
				}
			} else if fieldType.Kind() != reflect.Struct {
				continue
			}
			infos = append(infos, _InjectFieldInfo{
				Index:      i,
				IsEmbedded: true,
				Type:       field.Type,
			})
		}
	}
	return infos
}

func makeInjectStructFunc(targetType reflect.Type) _InjectStructFunc {
	t := targetType
	numDeref := 0
	for {
		if numDeref > 100 {
			panic(errWith(ErrBadArgument, "too many dereferences or recursive pointer type %v", t))
		}
		if t.Kind() == reflect.Pointer {
			numDeref++
			t = t.Elem()
			continue
		}
		if t.Kind() == reflect.Struct {
			break
		}
		panic(errWith(ErrBadArgument, "target type %v is not a struct or pointer to struct", t))
	}

	validateAcyclicEmbeddedStructs(t)
	infos := collectInjectFieldInfos(t)
	canFastMemo := numDeref == 1 && len(infos) == 1 && !infos[0].IsEmbedded

	return func(scope Scope, value reflect.Value, depth int) {
		if depth > 64 {
			panic(errWith(ErrBadArgument, "recursive struct injection depth limit exceeded"))
		}
		for range numDeref {
			if value.IsNil() {
				if value.CanSet() {
					value.Set(reflect.New(value.Type().Elem()))
				} else {
					throwNilPtrInjectTarget(value.Type())
				}
			}
			value = value.Elem()
		}
		for i := range infos {
			info := &infos[i]
			if info.IsEmbedded {
				injectEmbeddedField(scope, value.Field(info.Index), depth)
				continue
			}
			c := info.cache.Load()
			if c == nil || c.scope != scope {
				c = resolveInjectField(scope, info)
				info.cache.Store(c)
			}
			setReflectedField(value.Field(info.Index), c, info.IsSignedInt)
			if canFastMemo && depth == 0 {
				injectFastMemo.Store(&_InjectFastMemo{
					typ:        targetType,
					scope:      scope,
					rv:         c.rv,
					intVal:     c.intVal,
					fieldIndex: info.Index,
					isIntField: info.IsSignedInt,
				})
			}
		}
	}
}

func resolveInjectField(scope Scope, info *_InjectFieldInfo) *_InjectFieldCache {
	if info.IsInject {
		rv := info.Maker.makeInject(scope)
		return &_InjectFieldCache{scope: scope, rv: rv}
	}
	v, ok := scope.get(info.TypeID)
	if !ok {
		throwErrDependencyNotFound(info.Type)
	}
	var intVal int64
	if info.IsSignedInt {
		intVal = v.Int()
	}
	return &_InjectFieldCache{scope: scope, rv: v, intVal: intVal}
}

func injectEmbeddedField(scope Scope, fieldValue reflect.Value, depth int) {
	if fieldValue.Type().Kind() == reflect.Pointer {
		if fieldValue.IsNil() {
			if !fieldValue.CanSet() {
				return
			}
			fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
		}
		injectStruct(scope, fieldValue.Interface(), depth+1)
		return
	}
	if fieldValue.CanAddr() {
		injectStruct(scope, fieldValue.Addr().Interface(), depth+1)
	}
}
